package main

import (
	"log"
	"net"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"github.com/google/uuid"
	pluginapi "k8s.io/kubernetes/pkg/kubelet/apis/deviceplugin/v1beta1"
)

const (
	resourceName	= "takmatsu/fake"
	serverSock	= pluginapi.DevicePluginPath + "fake.sock"
)

type FakeDevicePlugin struct {
	devs []*pluginapi.Device
	socket string
	
	stop chan interface{}
	health chan *pluginapi.Device
	server *grpc.Server
}

func NewFakeDevicePlugin() *FakeDevicePlugin {
	return &FakeDevicePlugin {
		devs:	getDevices(),
		socket: serverSock,
	}
}

func (m *FakeDevicePlugin) GetDevicePluginOptions(context.Context, *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{}, nil
}

func dial(unixSocketPath string, timeout time.Duration) (*grpc.ClientConn, error) {
	c, err := grpc.Dial(unixSocketPath, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithTimeout(timeout),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			return net.DialTimeout("unix", addr, timeout)
		}),
	)

	if err != nil {
		return nil, err
	}

	return c, nil
}

func getDevices() []*pluginapi.Device {
	var devs []*pluginapi.Device
	for i := uint(0); i < 8; i++ {
		u, _ := uuid.NewRandom()
		devs = append(devs, &pluginapi.Device{
			ID:	u.String(),
			Health:	pluginapi.Healthy,
		})
	}
	return devs
}

func (m *FakeDevicePlugin) Start() error {
	err := m.cleanup()
	if err != nil {
		return err
	}

	sock, err := net.Listen("unix", m.socket)
	if err != nil {
		return err
	}

	m.server = grpc.NewServer([]grpc.ServerOption{}...)
	pluginapi.RegisterDevicePluginServer(m.server, m)

	go m.server.Serve(sock)

	// Wait for server to start by launching a blocking connexion
	conn, err := dial(m.socket, 5*time.Second)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

func (m *FakeDevicePlugin) Stop() error {
	if m.server == nil {
		return nil
	}

	m.server.Stop()
	m.server = nil
	close(m.stop)

	return m.cleanup()
}

func (m *FakeDevicePlugin) Register(kubeletEndpoint, resourceName string) error {
	conn, err := dial(kubeletEndpoint, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pluginapi.NewRegistrationClient(conn)
	reqt := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     path.Base(m.socket),
		ResourceName: resourceName,
	}

	_, err = client.Register(context.Background(), reqt)
	if err != nil {
		return err
	}
	return nil
}

func (m *FakeDevicePlugin) ListAndWatch(e *pluginapi.Empty, s pluginapi.DevicePlugin_ListAndWatchServer) error {
	s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})

	for {
		select {
		case <-m.stop:
			return nil
		case d := <-m.health:
			// FIXME: there is no way to recover from the Unhealthy state.
			d.Health = pluginapi.Unhealthy
			s.Send(&pluginapi.ListAndWatchResponse{Devices: m.devs})
		}
	}
}

func (m *FakeDevicePlugin) Allocate(ctx context.Context, reqs *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	responses := pluginapi.AllocateResponse{}
	for _, req := range reqs.ContainerRequests {
		response := pluginapi.ContainerAllocateResponse{
			Envs: map[string]string{
				"FAKE_DEVICES": strings.Join(req.DevicesIDs, ","),
			},
		}

		responses.ContainerResponses = append(responses.ContainerResponses, &response)
	}

	return &responses, nil
}

func (m *FakeDevicePlugin) PreStartContainer(context.Context, *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *FakeDevicePlugin) cleanup() error {
	if err := os.Remove(m.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (m *FakeDevicePlugin) Serve() error {
	err := m.Start()
	if err != nil {
		log.Printf("Could not start device plugin: %s", err)
		return err
	}
	log.Println("Starting to serve on", m.socket)

	err = m.Register(pluginapi.KubeletSocket, resourceName)
	if err != nil {
		log.Printf("Could not register device plugin: %s", err)
		m.Stop()
		return err
	}
	log.Println("Registered device plugin with Kubelet")

	return nil
}


