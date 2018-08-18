package provisioner

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/sugarkube/sugarkube/internal/pkg/vars"
)

type Provisioner interface {
	// Creates a cluster
	Create(sc *vars.StackConfig, values map[string]interface{}) error
	// Returns whether the cluster is already running
	IsOnline(sc *vars.StackConfig, values map[string]interface{}) (bool, error)
	// Update the cluster config if supported by the provisioner
	Update(sc *vars.StackConfig, values map[string]interface{}) error
}

// Factory that creates providers
func NewProvisioner(name string) (Provisioner, error) {
	if name == "minikube" {
		return MinikubeProvisioner{}, nil
	}

	if name == "kops" {
		return KopsProvisioner{}, nil
	}

	return nil, errors.New(fmt.Sprintf("Provider '%s' doesn't exist", name))
}
