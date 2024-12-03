package oci_test

import (
	"github.com/agoda-com/macOS-vz-kubelet/pkg/oci"
	"oras.land/oras-go/v2"
)

// check that custom OCI Store conforms to oras target interfaces
var _ oras.Target = &oci.Store{}
var _ oras.GraphTarget = &oci.Store{}
var _ oras.ReadOnlyTarget = &oci.Store{}
var _ oras.ReadOnlyGraphTarget = &oci.Store{}
