/*
 * Copyright 2018 The Sugarkube Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package provisioner

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/sugarkube/sugarkube/internal/pkg/clustersot"
	"github.com/sugarkube/sugarkube/internal/pkg/kapp"
	"github.com/sugarkube/sugarkube/internal/pkg/log"
	"github.com/sugarkube/sugarkube/internal/pkg/provider"
	"os"
	"os/exec"
	"strings"
)

type KopsProvisioner struct {
	clusterSot clustersot.ClusterSot
}

// todo - make configurable
const KOPS_PATH = "kops"

func (p KopsProvisioner) create(sc *kapp.StackConfig, providerImpl provider.Provider,
	dryRun bool) error {

	providerVars := provider.GetVars(providerImpl)
	log.Debugf("Creating stack with Kops and values: %#v", providerVars)

	args := make([]string, 0)
	args = append(args, "start")

	provisionerValues := providerVars[PROVISIONER_KEY].(map[interface{}]interface{})

	for k, v := range provisionerValues {
		key := strings.Replace(k.(string), "_", "-", -1)
		args = append(args, "--"+key)
		args = append(args, fmt.Sprintf("%v", v))
	}

	cmd := exec.Command(KOPS_PATH, args...)
	cmd.Env = os.Environ()

	if dryRun {
		log.Infof("Dry run. Skipping invoking Kops, but would execute: %s %s",
			KOPS_PATH, strings.Join(args, " "))
	} else {
		log.Infof("Launching Kops cluster... Executing: %s %s", KOPS_PATH,
			strings.Join(args, " "))

		err := cmd.Run()

		if err != nil {
			return errors.Wrap(err, "Failed to start a Kops cluster")
		}

		log.Infof("Kops cluster successfully started")
	}

	sc.Status.StartedThisRun = true
	// only sleep before checking the cluster fo readiness if we started it
	sc.Status.SleepBeforeReadyCheck = SLEEP_SECONDS_BEFORE_READY_CHECK

	return nil
}

func (p KopsProvisioner) ClusterSot() (clustersot.ClusterSot, error) {
	if p.clusterSot == nil {
		clusterSot, err := clustersot.NewClusterSot(clustersot.KUBECTL)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		p.clusterSot = clusterSot
	}

	return p.clusterSot, nil
}

func (p KopsProvisioner) isAlreadyOnline(sc *kapp.StackConfig, providerImpl provider.Provider) (bool, error) {
	panic("not implemented")
}

// No-op function, required to fully implement the Provisioner interface
func (p KopsProvisioner) update(sc *kapp.StackConfig, providerImpl provider.Provider) error {
	panic("not implemented")
}
