package ros_node

import (
	"fmt"
	"sync"

	"github.com/okieraised/monitoring-agent/internal/infrastructure/log"
	"github.com/okieraised/rclgo/jazzy"
)

var (
	nodeOnce sync.Once
	rosNode  *jazzy.Node
	nodeErr  error
)

func Node() *jazzy.Node {
	if rosNode == nil {
		log.Default().Fatal("monitoring agent ros node has not been initialized")
	}
	return rosNode
}

func NewRosNode() (*jazzy.Node, error) {
	nodeOnce.Do(func() {
		node, err := jazzy.NewNode("monitoring_agent", "")
		if err != nil {
			nodeErr = fmt.Errorf("failed to create ROS2 monitoring agent node: %v", err)
			log.Default().Error(nodeErr.Error())
			return
		}
		rosNode = node
	})

	return rosNode, nodeErr

}
