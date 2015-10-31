package docker

import (
	"crypto/md5"
	"errors"
	"fmt"
	"os"
	u "os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsouza/go-dockerclient"

	"bytes"
	"github.com/bssthu/gitlab-ci-multi-runner/common"
	"github.com/bssthu/gitlab-ci-multi-runner/executors"
	"github.com/bssthu/gitlab-ci-multi-runner/helpers"
	docker_helpers "github.com/bssthu/gitlab-ci-multi-runner/helpers/docker"
)

type DockerExecutor struct {
	executors.AbstractExecutor
	client         *docker.Client
	buildContainer *docker.Container
	services       []*docker.Container
	caches         []*docker.Container
}

func (s *DockerExecutor) getServiceVariables() []string {
	variables := s.Config.Environment

	for _, buildVariable := range s.Build.Variables {
		if !buildVariable.Public {
			continue
		}

		variable := fmt.Sprintf("%s=%s", buildVariable.Key, buildVariable.Value)
		variables = append(variables, variable)
	}

	return variables
}

func (s *DockerExecutor) getAuthConfig(imageName string) (docker.AuthConfiguration, error) {
	user, err := u.Current()
	if s.Shell.User != nil {
		user, err = u.Lookup(*s.Shell.User)
	}
	if err != nil {
		return docker.AuthConfiguration{}, err
	}

	indexName, _ := docker_helpers.SplitDockerImageName(imageName)

	authConfigs, err := docker_helpers.ReadDockerAuthConfigs(user.HomeDir)
	if err != nil {
		// ignore doesn't exist errors
		if os.IsNotExist(err) {
			err = nil
		}
		return docker.AuthConfiguration{}, err
	}

	authConfig := docker_helpers.ResolveDockerAuthConfig(indexName, authConfigs)
	if authConfig != nil {
		s.Debugln("Using", authConfig.Username, "to connect to", authConfig.ServerAddress, "in order to resolve", imageName, "...")
		return *authConfig, nil
	}

	return docker.AuthConfiguration{}, fmt.Errorf("No credentials found for %v", indexName)
}

func (s *DockerExecutor) getDockerImage(imageName string) (*docker.Image, error) {
	if !strings.Contains(imageName, ":") {
		imageName = imageName + ":latest"
	}

	s.Debugln("Looking for image", imageName, "...")
	image, err := s.client.InspectImage(imageName)
	if err == nil {
		if helpers.BoolOrDefault(s.Config.Docker.DisablePull, false) {
			return image, nil
		}
		if !pulledImageCache.isExpired(imageName) {
			return image, nil
		}
	}

	s.Println("Pulling docker image", imageName, "...")
	authConfig, err := s.getAuthConfig(imageName)
	if err != nil {
		s.Debugln(err)
	}

	pullImageOptions := docker.PullImageOptions{
		Repository: imageName,
	}

	err = s.client.PullImage(pullImageOptions, authConfig)
	if err != nil {
		if image != nil {
			s.Warningln("Cannot pull the latest version of image", imageName, ":", err)
			return image, nil
		} else {
			return nil, err
		}
	}

	image, err = s.client.InspectImage(imageName)
	if err != nil {
		return nil, err
	}

	pulledImageCache.mark(imageName, image.ID, dockerImageTTL)
	return image, nil
}

func (s *DockerExecutor) getAbsoluteContainerPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	} else {
		return filepath.Join(s.Build.FullProjectDir(), path)
	}
}

func (s *DockerExecutor) addHostVolume(binds *[]string, hostPath, containerPath string) error {
	containerPath = s.getAbsoluteContainerPath(containerPath)
	s.Debugln("Using host-based", hostPath, "for", containerPath, "...")
	*binds = append(*binds, fmt.Sprintf("%v:%v", hostPath, containerPath))
	return nil
}

func (s *DockerExecutor) getLabels(containerType string, otherLabels ...string) map[string]string {
	labels := make(map[string]string)
	labels[dockerLabelPrefix+".build.id"] = strconv.Itoa(s.Build.ID)
	labels[dockerLabelPrefix+".build.sha"] = s.Build.Sha
	labels[dockerLabelPrefix+".build.before_sha"] = s.Build.BeforeSha
	labels[dockerLabelPrefix+".build.ref_name"] = s.Build.RefName
	labels[dockerLabelPrefix+".project.id"] = strconv.Itoa(s.Build.ProjectID)
	labels[dockerLabelPrefix+".runner.id"] = s.Build.Runner.ShortDescription()
	labels[dockerLabelPrefix+".runner.global_id"] = strconv.Itoa(s.Build.GlobalID)
	labels[dockerLabelPrefix+".runner.local_id"] = strconv.Itoa(s.Build.RunnerID)
	labels[dockerLabelPrefix+".type"] = containerType
	for _, label := range otherLabels {
		keyValue := strings.SplitN(label, "=", 2)
		if len(keyValue) == 2 {
			labels[dockerLabelPrefix+"."+keyValue[0]] = keyValue[1]
		}
	}
	return labels
}

func (s *DockerExecutor) createCacheVolume(containerName, containerPath string) (*docker.Container, error) {
	// get busybox image
	cacheImage, err := s.getDockerImage("gitlab/gitlab-runner:cache")
	if err != nil {
		return nil, err
	}

	createContainerOptions := docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			Image: cacheImage.ID,
			Cmd: []string{
				containerPath,
			},
			Volumes: map[string]struct{}{
				containerPath: {},
			},
			Labels: s.getLabels("cache", "cache.dir="+containerPath),
		},
		HostConfig: &docker.HostConfig{},
	}

	container, err := s.client.CreateContainer(createContainerOptions)
	if err != nil {
		if container != nil {
			go s.removeContainer(container.ID)
		}
		return nil, err
	}

	s.Debugln("Starting cache container", container.ID, "...")
	err = s.client.StartContainer(container.ID, nil)
	if err != nil {
		go s.removeContainer(container.ID)
		return nil, err
	}

	s.Debugln("Waiting for cache container", container.ID, "...")
	errorCode, err := s.client.WaitContainer(container.ID)
	if err != nil {
		go s.removeContainer(container.ID)
		return nil, err
	}

	if errorCode != 0 {
		go s.removeContainer(container.ID)
		return nil, fmt.Errorf("cache container for %s returned %d", containerPath, errorCode)
	}

	return container, nil
}

func (s *DockerExecutor) addCacheVolume(binds, volumesFrom *[]string, containerPath string) error {
	var err error
	containerPath = s.getAbsoluteContainerPath(containerPath)

	// disable cache for automatic container cache, but leave it for host volumes (they are shared on purpose)
	if helpers.BoolOrDefault(s.Config.Docker.DisableCache, false) {
		s.Debugln("Container cache for", containerPath, " is disabled.")
		return nil
	}

	hash := md5.Sum([]byte(containerPath))

	// use host-based cache
	if cacheDir := helpers.StringOrDefault(s.Config.Docker.CacheDir, ""); cacheDir != "" {
		hostPath := fmt.Sprintf("%s/%s/%x", cacheDir, s.Build.ProjectUniqueName(), hash)
		hostPath, err := filepath.Abs(hostPath)
		if err != nil {
			return err
		}
		s.Debugln("Using path", hostPath, "as cache for", containerPath, "...")
		*binds = append(*binds, fmt.Sprintf("%v:%v", hostPath, containerPath))
		return nil
	}

	// get existing cache container
	containerName := fmt.Sprintf("%s-cache-%x", s.Build.ProjectUniqueName(), hash)
	container, _ := s.client.InspectContainer(containerName)

	// check if we have valid cache, if not remove the broken container
	if container != nil && container.Volumes[containerPath] == "" {
		s.removeContainer(container.ID)
		container = nil
	}

	// create new cache container for that project
	if container == nil {
		container, err = s.createCacheVolume(containerName, containerPath)
		if err != nil {
			return err
		}
	}

	s.Debugln("Using container", container.ID, "as cache", containerPath, "...")
	*volumesFrom = append(*volumesFrom, container.ID)
	return nil
}

func (s *DockerExecutor) addVolume(binds, volumesFrom *[]string, volume string) error {
	var err error
	hostVolume := strings.SplitN(volume, ":", 2)
	switch len(hostVolume) {
	case 2:
		err = s.addHostVolume(binds, hostVolume[0], hostVolume[1])

	case 1:
		// disable cache disables
		err = s.addCacheVolume(binds, volumesFrom, hostVolume[0])
	}

	if err != nil {
		s.Errorln("Failed to create container volume for", volume, err)
	}
	return err
}

func (s *DockerExecutor) createVolumes(image *docker.Image) ([]string, []string, error) {
	var binds, volumesFrom []string

	for _, volume := range s.Config.Docker.Volumes {
		s.addVolume(&binds, &volumesFrom, volume)
	}

	// Cache Git sources:
	// take path of the projects directory,
	// because we use `rm -rf` which could remove the mounted volume
	parentDir := filepath.Dir(s.Build.FullProjectDir())

	// Caching is supported only for absolute and non-root paths
	if filepath.IsAbs(parentDir) && parentDir != "/" {
		if s.Build.AllowGitFetch && !helpers.BoolOrDefault(s.Config.Docker.DisableCache, false) {
			// create persistent cache container
			s.addVolume(&binds, &volumesFrom, parentDir)
		} else {
			// create temporary cache container
			container, _ := s.createCacheVolume("", parentDir)
			if container != nil {
				s.caches = append(s.caches, container)
				volumesFrom = append(volumesFrom, container.ID)
			}
		}
	}

	return binds, volumesFrom, nil
}

func (s *DockerExecutor) splitServiceAndVersion(serviceDescription string) (string, string, string) {
	splits := strings.SplitN(serviceDescription, ":", 2)
	service := ""
	version := "latest"
	switch len(splits) {
	case 1:
		service = splits[0]

	case 2:
		service = splits[0]
		version = splits[1]

	default:
		return "", "", ""
	}

	linkName := strings.Replace(service, "/", "__", -1)
	return service, version, linkName
}

func (s *DockerExecutor) createService(service, version string) (*docker.Container, error) {
	if len(service) == 0 {
		return nil, errors.New("invalid service name")
	}

	serviceImage, err := s.getDockerImage(service + ":" + version)
	if err != nil {
		return nil, err
	}

	containerName := s.Build.ProjectUniqueName() + "-" + strings.Replace(service, "/", "__", -1)

	// this will fail potentially some builds if there's name collision
	s.removeContainer(containerName)

	s.Println("Starting service", service+":"+version, "...")
	createContainerOpts := docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			Image:  serviceImage.ID,
			Labels: s.getLabels("service", "service="+service, "service.version="+version),
			Env:    s.getServiceVariables(),
		},
		HostConfig: &docker.HostConfig{
			RestartPolicy: docker.NeverRestart(),
		},
	}

	s.Debugln("Creating service container", createContainerOpts.Name, "...")
	container, err := s.client.CreateContainer(createContainerOpts)
	if err != nil {
		return nil, err
	}

	s.Debugln("Starting service container", container.ID, "...")
	err = s.client.StartContainer(container.ID, nil)
	if err != nil {
		go s.removeContainer(container.ID)
		return nil, err
	}

	return container, nil
}

func (s *DockerExecutor) getServiceNames() ([]string, error) {
	services := s.Config.Docker.Services

	if servicesOption, ok := s.Build.Options["services"].([]interface{}); ok {
		for _, service := range servicesOption {
			serviceName, ok := service.(string)
			if !ok {
				s.Errorln("Invalid service name passed:", service)
				return nil, errors.New("invalid service name")
			}

			err := s.verifyAllowedImage(serviceName, "services", s.Config.Docker.AllowedServices, s.Config.Docker.Services)
			if err != nil {
				return nil, err
			}

			services = append(services, serviceName)
		}
	}

	return services, nil
}

func (s *DockerExecutor) createServices() ([]string, error) {
	serviceNames, err := s.getServiceNames()
	if err != nil {
		return nil, err
	}

	linksMap := make(map[string]*docker.Container)

	for _, serviceDescription := range serviceNames {
		service, version, linkName := s.splitServiceAndVersion(serviceDescription)
		if linksMap[linkName] != nil {
			s.Warningln("Service", serviceDescription, "is already created. Ignoring.")
			continue
		}

		container, err := s.createService(service, version)
		if err != nil {
			return nil, err
		}

		s.Debugln("Created service", serviceDescription, "as", container.ID)
		linksMap[linkName] = container
		s.services = append(s.services, container)
	}

	waitForServicesTimeout := common.DefaultWaitForServicesTimeout
	if s.Config.Docker.WaitForServicesTimeout != nil {
		waitForServicesTimeout = *s.Config.Docker.WaitForServicesTimeout
	}

	// wait for all services to came up
	if waitForServicesTimeout > 0 && len(s.services) > 0 {
		s.Println("Waiting for services to be up and running...")
		wg := sync.WaitGroup{}
		for _, service := range s.services {
			wg.Add(1)
			go func(service *docker.Container) {
				s.waitForServiceContainer(service, time.Duration(waitForServicesTimeout)*time.Second)
				wg.Done()
			}(service)
		}
		wg.Wait()
	}

	var links []string
	for linkName, container := range linksMap {
		newContainer, err := s.client.InspectContainer(container.ID)
		if err != nil {
			continue
		}
		if newContainer.State.Running {
			links = append(links, container.ID+":"+linkName)
		}
	}

	return links, nil
}

func (s *DockerExecutor) connect() (*docker.Client, error) {
	endpoint := "unix:///var/run/docker.sock"
	tlsVerify := false
	tlsCertPath := ""

	if host := helpers.StringOrDefault(s.Config.Docker.Host, ""); host != "" {
		// read docker config from config
		endpoint = host
		if s.Config.Docker.CertPath != nil {
			tlsVerify = true
			tlsCertPath = *s.Config.Docker.CertPath
		}
	} else if host := os.Getenv("DOCKER_HOST"); host != "" {
		// read docker config from environment
		endpoint = host
		tlsVerify, _ = strconv.ParseBool(os.Getenv("DOCKER_TLS_VERIFY"))
		tlsCertPath = os.Getenv("DOCKER_CERT_PATH")
	}

	if tlsVerify {
		client, err := docker.NewVersionnedTLSClient(
			endpoint,
			filepath.Join(tlsCertPath, "cert.pem"),
			filepath.Join(tlsCertPath, "key.pem"),
			filepath.Join(tlsCertPath, "ca.pem"),
			dockerAPIVersion,
		)
		if err != nil {
			return nil, err
		}

		return client, nil
	} else {
		client, err := docker.NewVersionedClient(endpoint, dockerAPIVersion)
		if err != nil {
			return nil, err
		}

		return client, nil
	}
}

func (s *DockerExecutor) createBuildContainer(cmd []string) error {
	hostname := helpers.StringOrDefault(s.Config.Docker.Hostname, s.Build.ProjectUniqueName())
	containerName := s.Build.ProjectUniqueName()

	// this will fail potentially some builds if there's name collision
	s.removeContainer(containerName)

	imageName, err := s.getImageName()
	if err != nil {
		return err
	}

	image, err := s.getDockerImage(imageName)
	if err != nil {
		return err
	}

	createContainerOptions := docker.CreateContainerOptions{
		Name: containerName,
		Config: &docker.Config{
			Hostname:     hostname,
			Image:        image.ID,
			Tty:          false,
			AttachStdin:  true,
			AttachStdout: true,
			AttachStderr: true,
			OpenStdin:    true,
			StdinOnce:    true,
			Env:          s.ShellScript.Environment,
			Cmd:          cmd,
			Labels:       s.getLabels("build"),
		},
		HostConfig: &docker.HostConfig{
			Privileged:    s.Config.Docker.Privileged,
			RestartPolicy: docker.NeverRestart(),
			ExtraHosts:    s.Config.Docker.ExtraHosts,
			Links:         s.Config.Docker.Links,
		},
	}

	s.Debugln("Creating services...")
	links, err := s.createServices()
	if err != nil {
		return err
	}
	createContainerOptions.HostConfig.Links = append(createContainerOptions.HostConfig.Links, links...)

	s.Debugln("Creating cache directories...")
	binds, volumesFrom, err := s.createVolumes(image)
	if err != nil {
		return err
	}
	createContainerOptions.HostConfig.Binds = binds
	createContainerOptions.HostConfig.VolumesFrom = volumesFrom

	s.Debugln("Creating container", createContainerOptions.Name, "...")
	container, err := s.client.CreateContainer(createContainerOptions)
	if err != nil {
		if container != nil {
			go s.removeContainer(container.ID)
		}
		return err
	}

	s.Debugln("Starting container", container.ID, "...")
	err = s.client.StartContainer(container.ID, nil)
	if err != nil {
		go s.removeContainer(container.ID)
		return err
	}

	s.buildContainer = container
	return nil
}

func (s *DockerExecutor) removeContainer(id string) error {
	removeContainerOptions := docker.RemoveContainerOptions{
		ID:            id,
		RemoveVolumes: true,
		Force:         true,
	}
	err := s.client.RemoveContainer(removeContainerOptions)
	s.Debugln("Removed container", id, "with", err)
	return err
}

func (s *DockerExecutor) verifyAllowedImage(image, optionName string, allowedImages []string, internalImages []string) error {
	for _, allowedImage := range allowedImages {
		ok, _ := filepath.Match(allowedImage, image)
		if ok {
			return nil
		}
	}

	for _, internalImage := range internalImages {
		if internalImage == image {
			return nil
		}
	}

	s.Println()
	if len(allowedImages) != 0 {
		s.Errorln("The", image, "is not present on list of allowed", optionName)
		for _, allowedImage := range allowedImages {
			s.Println("-", allowedImage)
		}
		s.Println()
	} else {
		// by default allow to override the image name
		return nil
	}

	s.Println("Please check runner's configuration: http://doc.gitlab.com/ci/docker/using_docker_images.html#overwrite-image-and-services")
	return errors.New("invalid image")
}

func (s *DockerExecutor) getImageName() (string, error) {
	if imageOption, ok := s.Build.Options["image"].(string); ok && imageOption != "" {
		err := s.verifyAllowedImage(imageOption, "images", s.Config.Docker.AllowedImages, []string{s.Config.Docker.Image})
		if err != nil {
			return "", err
		}
		return imageOption, nil
	}

	return s.Config.Docker.Image, nil
}

func (s *DockerExecutor) Prepare(globalConfig *common.Config, config *common.RunnerConfig, build *common.Build) error {
	err := s.AbstractExecutor.Prepare(globalConfig, config, build)
	if err != nil {
		return err
	}

	if s.ShellScript.PassFile {
		return errors.New("Docker doesn't support shells that require script file")
	}

	if config.Docker == nil {
		return errors.New("Missing docker configuration")
	}

	imageName, err := s.getImageName()
	if err != nil {
		return err
	}
	s.Println("Using Docker executor with image", imageName, "...")

	client, err := docker_helpers.Connect(s.Config.Docker.DockerCredentials, dockerAPIVersion)
	if err != nil {
		return err
	}
	s.client = client
	return nil
}

func (s *DockerExecutor) Cleanup() {
	for _, service := range s.services {
		s.removeContainer(service.ID)
	}

	for _, cache := range s.caches {
		s.removeContainer(cache.ID)
	}

	if s.buildContainer != nil {
		s.removeContainer(s.buildContainer.ID)
		s.buildContainer = nil
	}

	s.AbstractExecutor.Cleanup()
}

func (s *DockerExecutor) runServiceHealthCheckContainer(container *docker.Container, timeout time.Duration) error {
	waitImage, err := s.getDockerImage("gitlab/gitlab-runner:service")
	if err != nil {
		return err
	}

	waitContainerOpts := docker.CreateContainerOptions{
		Name: container.Name + "-wait-for-service",
		Config: &docker.Config{
			Image:  waitImage.ID,
			Labels: s.getLabels("wait", "wait="+container.ID),
		},
		HostConfig: &docker.HostConfig{
			RestartPolicy: docker.NeverRestart(),
			Links:         []string{container.Name + ":" + container.Name},
		},
	}
	s.Debugln("Waiting for service container", container.Name, "to be up and running...")
	waitContainer, err := s.client.CreateContainer(waitContainerOpts)
	if err != nil {
		return err
	}
	defer s.removeContainer(waitContainer.ID)
	err = s.client.StartContainer(waitContainer.ID, nil)
	if err != nil {
		return err
	}

	waitResult := make(chan error, 1)
	go func() {
		statusCode, err := s.client.WaitContainer(waitContainer.ID)
		if err == nil && statusCode != 0 {
			err = fmt.Errorf("Status code: %d", statusCode)
		}
		waitResult <- err
	}()

	// these are warnings and they don't make the build fail
	select {
	case err := <-waitResult:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("didn't respond in timely maner: %v (consider modifying wait_for_services_timeout).", container.Name, timeout)
	}
	return nil
}

func (s *DockerExecutor) waitForServiceContainer(container *docker.Container, timeout time.Duration) error {
	err := s.runServiceHealthCheckContainer(container, timeout)
	if err == nil {
		return nil
	}

	var buffer bytes.Buffer
	buffer.WriteString("\n")
	buffer.WriteString(helpers.ANSI_BOLD_YELLOW + "*** WARNING:" + helpers.ANSI_RESET + " Service " + container.Name + " probably didn't start properly.\n")
	buffer.WriteString("\n")
	buffer.WriteString(strings.TrimSpace(err.Error()) + "\n")

	var containerBuffer bytes.Buffer

	err = s.client.Logs(docker.LogsOptions{
		Container:    container.ID,
		OutputStream: &containerBuffer,
		ErrorStream:  &containerBuffer,
		Stdout:       true,
		Stderr:       true,
		Timestamps:   true,
	})
	if err == nil {
		if containerLog := containerBuffer.String(); containerLog != "" {
			buffer.WriteString("\n")
			buffer.WriteString(strings.TrimSpace(containerLog))
			buffer.WriteString("\n")
		}
	} else {
		buffer.WriteString(strings.TrimSpace(err.Error()) + "\n")
	}

	buffer.WriteString("\n")
	buffer.WriteString(helpers.ANSI_BOLD_YELLOW + "*********" + helpers.ANSI_RESET + "\n")
	buffer.WriteString("\n")
	s.Build.WriteString(buffer.String())
	return err
}
