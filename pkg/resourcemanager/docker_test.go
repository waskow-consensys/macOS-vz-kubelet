package resourcemanager_test

import "github.com/agoda-com/macOS-vz-kubelet/pkg/resourcemanager"

// check that DockerClient implements the ContainersClient interface
var _ resourcemanager.ContainersClient = &resourcemanager.DockerClient{}
