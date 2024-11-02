// Package main provides GPU-enabled Docker container management through Redis pub/sub
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var log *zap.SugaredLogger

type Command struct {
	Action string            `json:"action"`
	ID     string            `json:"id"`
	Env    map[string]string `json:"env,omitempty"`
}

// DockerMonitor manages Docker containers based on Redis pub/sub messages.
// All operations that modify container state are protected by mutex.
type DockerMonitor struct {
	client       *client.Client
	redis        *redis.Client
	containerMap map[string]string // Maps custom IDs to Docker container IDs
	image        string
	channel      string
	mu           sync.RWMutex
}

func init() {
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.DisableCaller = true
	config.DisableStacktrace = true

	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}
	log = logger.Sugar()
}

// NewDockerMonitor creates a monitor instance configured for NVIDIA GPU support.
//
// The image parameter must reference a Docker image compatible with the NVIDIA runtime.
// Call Start to begin processing container commands.
func NewDockerMonitor(redisURL string, image string, channel string) (*DockerMonitor, error) {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %v", err)
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis URL: %v", err)
	}
	rdb := redis.NewClient(opt)

	return &DockerMonitor{
		client:       cli,
		redis:        rdb,
		containerMap: make(map[string]string),
		image:        image,
		channel:      channel,
	}, nil
}

// Start begins processing Redis messages. It returns immediately and processes
// messages until the context is cancelled.
//
// The caller must wait on the provided WaitGroup after calling Start.
func (dm *DockerMonitor) Start(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer dm.client.Close()
		defer dm.redis.Close()

		dm.listenToRedis(ctx)
	}()
}

func (dm *DockerMonitor) listenToRedis(ctx context.Context) {
	pubsub := dm.redis.Subscribe(ctx, dm.channel)
	defer pubsub.Close()

	log.Infow("Listening for messages",
		"channel", dm.channel)

	_, err := pubsub.Receive(ctx)
	if err != nil {
		log.Errorw("Error receiving subscription confirmation",
			"error", err)
		return
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Info("Shutting down...")
			return
		case msg := <-ch:
			go func(message *redis.Message) {
				log.Debugw("Received message",
					"payload", message.Payload)
				dm.handleRedisMessage(ctx, message.Payload)
			}(msg)
		}
	}
}

func (dm *DockerMonitor) handleRedisMessage(ctx context.Context, payload string) {
	var cmd Command
	if err := json.Unmarshal([]byte(payload), &cmd); err != nil {
		log.Errorw("Failed to parse command",
			"error", err,
			"payload", payload)
		return
	}

	switch cmd.Action {
	case "start":
		if cmd.Env == nil {
			cmd.Env = make(map[string]string)
		}
		log.Infow("Starting container",
			"id", cmd.ID,
			"env", cmd.Env)
		dm.startContainer(ctx, cmd.ID, cmd.Env)
	case "kill":
		log.Infow("Killing container",
			"id", cmd.ID)
		dm.killContainer(ctx, cmd.ID)
	default:
		log.Warnw("Unknown action received",
			"action", cmd.Action,
			"id", cmd.ID)
	}
}

// startContainer creates and starts a new container with the specified ID and environment.
// If a container with the given ID already exists, the operation is ignored.
//
// The container is automatically configured with NVIDIA GPU support and access to all
// available GPUs on the host. Environment variables are passed through to the container.
func (dm *DockerMonitor) startContainer(ctx context.Context, id string, env map[string]string) {
	dm.mu.Lock()
	if _, exists := dm.containerMap[id]; exists { // if container EXISTS
		dm.mu.Unlock()
		log.Warnw("Container already running", "id", id)
		return
	}
	dm.mu.Unlock()

	envArray := make([]string, 0, len(env)+1)
	envArray = append(envArray, "NVIDIA_DRIVER_CAPABILITIES=all")
	for key, value := range env {
		envArray = append(envArray, fmt.Sprintf("%s=%s", key, value))
	}

	deviceRequests := []container.DeviceRequest{
		{
			Driver:       "nvidia",
			Count:       -1,
			Capabilities: [][]string{{"gpu", "compute", "utility"}},
		},
	}

	resp, err := dm.client.ContainerCreate(ctx, 
		&container.Config{
			Image: dm.image,
			Env:   envArray,
		}, 
		&container.HostConfig{
			Runtime: "nvidia",
			Resources: container.Resources{
				DeviceRequests: deviceRequests,
			},
		},
		nil, nil, "")
	if err != nil {
		log.Errorw("Failed to create container",
			"error", err,
			"id", id)
		return
	}

	if err := dm.client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
		log.Errorw("Failed to start container",
			"error", err,
			"id", id,
			"container_id", resp.ID)
		return
	}

	dm.mu.Lock()
	dm.containerMap[id] = resp.ID
	dm.mu.Unlock()

	log.Infow("Container started successfully",
		"id", id,
		"container_id", resp.ID)
}

// killContainer stops and removes a container. 
// If the container doesn't exist, the operation is ignored.
// The operation is synchronous - it blocks until the container is fully removed.
func (dm *DockerMonitor) killContainer(ctx context.Context, id string) {
	dm.mu.Lock()
	containerID, exists := dm.containerMap[id]
	if !exists { // if NO container found
		dm.mu.Unlock()
		log.Warnw("No container found",
			"id", id)
		return
	}
	dm.mu.Unlock()

	if err := dm.client.ContainerStop(ctx, containerID, container.StopOptions{}); err != nil {
		log.Errorw("Failed to stop container",
			"error", err,
			"id", id,
			"container_id", containerID)
		return
	}

	if err := dm.client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{}); err != nil {
		log.Errorw("Failed to remove container",
			"error", err,
			"id", id,
			"container_id", containerID)
		return
	}

	dm.mu.Lock()
	delete(dm.containerMap, id)
	dm.mu.Unlock()

	log.Infow("Container stopped and removed successfully",
		"id", id,
		"container_id", containerID)
}

func main() {
	redisURL := flag.String("redis", "redis://localhost:6379/0", "Redis URL")
	image := flag.String("image", "", "Docker image to run")
	channel := flag.String("channel", "bot-workers", "Redis channel to subscribe to")
	flag.Parse()

	if *image == "" {
		log.Fatal("--image flag is required")
	}

	defer log.Sync()

	ctx := context.Background()
	var wg sync.WaitGroup

	monitor, err := NewDockerMonitor(*redisURL, *image, *channel)
	if err != nil {
		log.Fatalw("Failed to create Docker monitor",
			"error", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	defer func() {
		signal.Stop(c)
		cancel()
	}()

	go func() {
		select {
		case <-c:
			log.Info("Interrupt received, shutting down...")
			cancel()
		case <-ctx.Done():
		}
	}()

	monitor.Start(ctx, &wg)
	wg.Wait()
}