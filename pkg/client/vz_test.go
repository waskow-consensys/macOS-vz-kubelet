package client_test

import (
	"github.com/agoda-com/macOS-vz-kubelet/pkg/client"
)

// check that VzClientAPIs implements the VzClientInterface interface
var _ client.VzClientInterface = &client.VzClientAPIs{}
