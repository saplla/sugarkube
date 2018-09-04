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
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sugarkube/sugarkube/internal/pkg/clustersot"
	"github.com/sugarkube/sugarkube/internal/pkg/kapp"
	"github.com/sugarkube/sugarkube/internal/pkg/log"
	"github.com/sugarkube/sugarkube/internal/pkg/provider"
	"os"
	"os/exec"
	"strings"
	"time"
)

type KopsProvisioner struct {
	clusterSot clustersot.ClusterSot
}

// todo - make configurable
const KOPS_PATH = "kops"

const SPEC_KEY = "spec"

// Returns whether a kops cluster config has already been created (this doesn't check whether the cluster is actually
// running though).
func (p KopsProvisioner) clusterConfigExists(sc *kapp.StackConfig, providerImpl provider.Provider) (bool, error) {

	providerVars := provider.GetVars(providerImpl)
	log.Debugf("Checking if Kops cluster config exists for values: %#v", providerVars)

	provisionerValues := providerVars[PROVISIONER_KEY].(map[interface{}]interface{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	args := []string{
		"get",
		"clusters",
		"--state",
		provisionerValues["state"].(string),
		provisionerValues["name"].(string),
	}

	cmd := exec.CommandContext(ctx, KOPS_PATH, args...)
	cmd.Env = os.Environ()

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return false, errors.New("Timed out trying to retrieve kops cluster config. Check your credentials.")
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			log.Debug("Cluster config doesn'te exist")
			return false, nil
		} else {
			return false, errors.Wrap(err, "Error fetching kops clusters")
		}
	}

	return true, nil
}

func (p KopsProvisioner) create(sc *kapp.StackConfig, providerImpl provider.Provider,
	dryRun bool) error {

	providerVars := provider.GetVars(providerImpl)
	log.Debugf("Creating stack with Kops and values: %#v", providerVars)

	args := make([]string, 0)
	args = append(args, "create", "cluster")

	provisionerValues := providerVars[PROVISIONER_KEY].(map[interface{}]interface{})

	ignoreKeys := []string{
		SPEC_KEY,
	}

	for k, v := range provisionerValues {
		key := strings.Replace(k.(string), "_", "-", -1)

		shouldIgnore := false

		for _, ignoreKey := range ignoreKeys {
			if key == ignoreKey {
				shouldIgnore = true
			}
		}

		if !shouldIgnore {
			args = append(args, "--"+key)
			args = append(args, fmt.Sprintf("%v", v))
		}
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	cmd := exec.Command(KOPS_PATH, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if dryRun {
		log.Infof("Dry run. Skipping invoking Kops, but would execute: %s %s",
			KOPS_PATH, strings.Join(args, " "))
	} else {
		log.Infof("Creating Kops cluster config... Executing: %s %s", KOPS_PATH,
			strings.Join(args, " "))

		err := cmd.Run()

		if err != nil {
			return errors.Wrapf(err, "Failed to create a Kops cluster config: %s", stderrBuf.String())
		}

		log.Debugf("Kops returned:\n%s", stdoutBuf.String())
		log.Infof("Kops cluster config created")
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
	configExists, err := p.clusterConfigExists(sc, providerImpl)
	if err != nil {
		return false, errors.WithStack(err)
	}

	if !configExists {
		return false, nil
	}

	panic("not implemented - need to try requesting namespaces or something via kubectl")
}

// No-op function, required to fully implement the Provisioner interface
func (p KopsProvisioner) update(sc *kapp.StackConfig, providerImpl provider.Provider) error {
	configExists, err := p.clusterConfigExists(sc, providerImpl)
	if err != nil {
		return errors.WithStack(err)
	}

	if !configExists {
		return nil
	}

	panic("not implemented")
}
