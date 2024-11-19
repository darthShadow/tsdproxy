package containers

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

const (
	LabelPrefix    = "tsdproxy."
	LabelIsEnabled = LabelEnable + "=true"

	// Container config labels
	LabelEnable        = LabelPrefix + "enable"
	LabelName          = LabelPrefix + "name"
	LabelContainerIP   = LabelPrefix + "container_ip"
	LabelContainerPort = LabelPrefix + "container_port"
	LabelEphemeral     = LabelPrefix + "ephemeral"
	LabelWebClient     = LabelPrefix + "webclient"
	LabelTsnetVerbose  = LabelPrefix + "tsnet_verbose"
	LabelFunnel        = LabelPrefix + "funnel"
	LabelAuthKey       = LabelPrefix + "authkey"
	LabelAuthKeyFile   = LabelPrefix + "authkeyfile"
)

type Container struct {
	Info           types.ContainerJSON
	ID             string
	TargetHostname string
	Labels         labels
}

type labels struct {
	Authkey      string
	Ephemeral    bool
	WebClient    bool
	TsnetVerbose bool
	Funnel       bool
}

func NewContainer(ctx context.Context, containerID string, docker *client.Client, hostname string, defaultAuthkey string) (*Container, error) {
	// Get the container info
	containerInfo, err := docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("error inspecting container: %w", err)
	}

	container := &Container{
		Info: containerInfo,
		ID:   containerID,
	}

	container.TargetHostname = container.getTargetHostname(hostname)

	container.Labels.Ephemeral = container.getLabelBool(LabelEphemeral, true)
	container.Labels.WebClient = container.getLabelBool(LabelWebClient, false)
	container.Labels.TsnetVerbose = container.getLabelBool(LabelTsnetVerbose, false)
	container.Labels.Funnel = container.getLabelBool(LabelFunnel, false)
	container.Labels.Authkey = container.getLabelString(LabelAuthKey, defaultAuthkey)
	if err := container.setAuthKeyFromAuthFile(); err != nil {
		return nil, fmt.Errorf("error setting auth key from file : %w", err)
	}
	return container, nil
}

func (c *Container) GetName() string {
	return strings.TrimLeft(c.Info.Name, "/")
}

func (c *Container) GetPort() (string, bool) {
	// If Label is defined, get the container port
	//
	if customContainerPort, ok := c.Info.Config.Labels[LabelContainerPort]; ok {
		return customContainerPort, true
	}

	for _, bind := range c.Info.NetworkSettings.Ports {
		if len(bind) > 0 {
			return bind[0].HostPort, true
		}
	}

	return "", false
}

func (c *Container) getTargetHostname(hostname string) string {
	// If Label is defined, get the container IP
	//
	if customContainerIP, ok := c.Info.Config.Labels[LabelContainerIP]; ok && customContainerIP != "" {
		return customContainerIP
	}

	// return container IP address if defined
	if len(c.Info.NetworkSettings.IPAddress) > 0 {
		return c.Info.NetworkSettings.IPAddress
	}

	// return the first IP address of the container
	if len(c.Info.NetworkSettings.Networks) > 0 {
		for _, network := range c.Info.NetworkSettings.Networks {
			if len(network.IPAddress) > 0 {
				return network.IPAddress
			}
		}
	}

	// return localhost if container same as host to serve the dashboard
	//
	osname, err := os.Hostname()
	if err != nil {
		return hostname
	}
	if strings.HasPrefix(c.Info.ID, osname) {
		return "127.0.0.1"
	}

	// return hostname defined in the config
	return hostname
}

func (c *Container) GetTargetURL() (*url.URL, error) {
	// Set default proxy URL (virtual server in Tailscale)

	containerPort, ok := c.GetPort()
	if !ok {
		return nil, fmt.Errorf("no port found in container")
	}

	return url.Parse(fmt.Sprintf("http://%s:%s", c.TargetHostname, containerPort))
}

func (c *Container) GetProxyURL() (*url.URL, error) {
	// set default proxy URL
	name := c.GetName()

	// Set custom proxy URL if present the Label in the container
	if customName, ok := c.Info.Config.Labels[LabelName]; ok {
		name = customName
	}

	// validate url
	//
	return url.Parse(fmt.Sprintf("https://%s", name))
}

func (c *Container) getLabelBool(label string, defaultValue bool) bool {
	// Set default value
	value := defaultValue
	if valueString, ok := c.Info.Config.Labels[label]; ok {
		valueBool, err := strconv.ParseBool(valueString)
		// set value only if no error
		// if error, keep default
		//
		if err == nil {
			value = valueBool
		}
	}
	return value
}

func (c *Container) getLabelString(label string, defaultValue string) string {
	// Set default value
	value := defaultValue
	if valueString, ok := c.Info.Config.Labels[label]; ok {
		value = valueString
	}
	return value
}

func (c *Container) setAuthKeyFromAuthFile() error {
	authKeyFile, ok := c.Info.Config.Labels[LabelAuthKeyFile]
	if !ok || authKeyFile == "" {
		// authkeyfile label not defined
		return nil
	}
	authKey, err := os.ReadFile(authKeyFile)
	if err != nil {
		return fmt.Errorf("read auth key from file: %w", err)
	}
	c.Labels.Authkey = strings.TrimSpace(string(authKey))
	return nil
}
