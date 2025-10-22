package webrtc

import (
	"context"
	"sync"

	"github.com/okieraised/rclgo/jazzy"
)

type topicBroadcaster struct {
	mu      sync.RWMutex
	topics  map[string]chan []byte
	viewers map[string][]chan<- []byte
	cancel  map[string]context.CancelFunc
}

func newTopicBroadcaster() *topicBroadcaster {
	return &topicBroadcaster{
		topics:  make(map[string]chan []byte),
		viewers: make(map[string][]chan<- []byte),
		cancel:  make(map[string]context.CancelFunc),
	}
}

// AddViewer adds a new viewer to a topic (fan-out)
func (b *topicBroadcaster) AddViewer(ctx context.Context, node *jazzy.Node, topic string, out chan<- []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.viewers[topic] = append(b.viewers[topic], out)

	if _, ok := b.topics[topic]; ok {
		return
	}

	frameCh := make(chan []byte, 64)
	b.topics[topic] = frameCh

	subCtx, cancel := context.WithCancel(ctx)
	b.cancel[topic] = cancel

	// Start the ROS subscriber node once
	go func() {
		enable := make(chan bool, 1)
		enable <- true
		_ = newCameraSubscriber(subCtx, node, topic, frameCh, enable)
	}()

	// Start the fan out loop
	go func() {
		for {
			select {
			case <-subCtx.Done():
				return
			case frame, ok := <-frameCh:
				if !ok {
					return
				}
				b.mu.RLock()
				for _, v := range b.viewers[topic] {
					select {
					case v <- frame:
					default:
					}
				}
				b.mu.RUnlock()
			}
		}
	}()
}

func (b *topicBroadcaster) RemoveViewer(topic string, out chan<- []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	lst := b.viewers[topic]
	for i, v := range lst {
		if v == out {
			lst = append(lst[:i], lst[i+1:]...)
			break
		}
	}
	b.viewers[topic] = lst

	if len(lst) == 0 {
		if cancel, ok := b.cancel[topic]; ok {
			cancel()
			delete(b.cancel, topic)
		}
		delete(b.topics, topic)
	}
}
