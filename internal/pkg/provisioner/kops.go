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
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/sugarkube/sugarkube/internal/pkg/clustersot"
	"github.com/sugarkube/sugarkube/internal/pkg/convert"
	"github.com/sugarkube/sugarkube/internal/pkg/kapp"
	"github.com/sugarkube/sugarkube/internal/pkg/log"
	"github.com/sugarkube/sugarkube/internal/pkg/provider"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"
)

type KopsProvisioner struct {
	clusterSot clustersot.ClusterSot
}

type KopsConfig struct {
	Params struct {
		Global        map[string]string
		CreateCluster map[string]string `yaml:"create_cluster"`
		RollingUpdate map[string]string `yaml:"rolling_update"`
	}
}

// todo - make configurable
const KOPS_PATH = "kops"

const SPECS_KEY = "specs"

//const KOPS_CREATE_CLUSTER_KEY = "create_cluster"
//const KOPS_ROLLING_UPDATE_KEY = "rolling_update"

const KOPS_SLEEP_SECONDS_BEFORE_READY_CHECK = 60

// Returns whether a kops cluster config has already been created (this doesn't check whether the cluster is actually
// running though).
func (p KopsProvisioner) clusterConfigExists(sc *kapp.StackConfig, providerImpl provider.Provider) (bool, error) {

	providerVars := provider.GetVars(providerImpl)
	log.Debugf("Checking if Kops cluster config exists for values: %#v", providerVars)

	provisionerValues := providerVars[PROVISIONER_KEY].(map[interface{}]interface{})
	kopsConfig, err := getKopsConfig(provisionerValues)
	if err != nil {
		return false, errors.WithStack(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	args := []string{
		"get",
		"clusters",
	}

	args = parameteriseValues(args, kopsConfig.Params.Global)

	cmd := exec.CommandContext(ctx, KOPS_PATH, args...)
	cmd.Env = os.Environ()

	err = cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return false, errors.New("Timed out trying to retrieve kops cluster config. Check your credentials.")
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			log.Debug("Cluster config doesn't exist")
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
	kopsConfig, err := getKopsConfig(provisionerValues)
	if err != nil {
		return errors.WithStack(err)
	}

	args = parameteriseValues(args, kopsConfig.Params.Global)
	args = parameteriseValues(args, kopsConfig.Params.CreateCluster)

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

	err = p.patch(sc, providerImpl, dryRun)
	if err != nil {
		return errors.WithStack(err)
	}

	sc.Status.StartedThisRun = true
	// only sleep before checking the cluster fo readiness if we started it
	sc.Status.SleepBeforeReadyCheck = KOPS_SLEEP_SECONDS_BEFORE_READY_CHECK

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

	clusterSot, err := p.ClusterSot()
	if err != nil {
		return false, errors.WithStack(err)
	}

	online, err := clustersot.IsOnline(clusterSot, sc, providerImpl)
	if err != nil {
		return false, errors.WithStack(err)
	}

	return online, nil
}

// Patches a Kops cluster config then performs a rolling update to apply it
func (p KopsProvisioner) update(sc *kapp.StackConfig, providerImpl provider.Provider,
	dryRun bool) error {

	err := p.patch(sc, providerImpl, dryRun)
	if err != nil {
		return errors.WithStack(err)
	}

	providerVars := provider.GetVars(providerImpl)

	provisionerValues := providerVars[PROVISIONER_KEY].(map[interface{}]interface{})
	kopsConfig, err := getKopsConfig(provisionerValues)
	if err != nil {
		return errors.WithStack(err)
	}

	log.Infof("Performing a rolling update to apply config changes to the kops cluster...")
	// todo make the --yes flag configurable, perhaps through a CLI arg so people can verify their
	// changes before applying them
	args := []string{
		"rolling-update",
		"cluster",
		"--yes",
	}

	args = parameteriseValues(args, kopsConfig.Params.Global)
	args = parameteriseValues(args, kopsConfig.Params.RollingUpdate)

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
		log.Infof("Running Kops rolling update... Executing: %s %s", KOPS_PATH,
			strings.Join(args, " "))

		err := cmd.Run()
		if err != nil {
			return errors.Wrapf(err, "Failed to update Kops cluster: %s", stderrBuf.String())
		}

		log.Debugf("Kops returned:\n%s", stdoutBuf.String())
		log.Infof("Kops cluster updated")
	}

	return nil
}

// Patches a Kops cluster configuration. Downloads the current config then merges in any configured
// spec.
func (p KopsProvisioner) patch(sc *kapp.StackConfig, providerImpl provider.Provider,
	dryRun bool) error {
	configExists, err := p.clusterConfigExists(sc, providerImpl)
	if err != nil {
		return errors.WithStack(err)
	}

	if !configExists {
		return nil
	}

	providerVars := provider.GetVars(providerImpl)
	provisionerValues := providerVars[PROVISIONER_KEY].(map[interface{}]interface{})
	kopsConfig, err := getKopsConfig(provisionerValues)
	if err != nil {
		return errors.WithStack(err)
	}

	// get the kops config
	args := []string{
		"get",
		"cluster",
		"-o",
		"yaml",
	}

	args = parameteriseValues(args, kopsConfig.Params.Global)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	cmd := exec.CommandContext(ctx, KOPS_PATH, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Debug("Downloading config for kops cluster...")

	err = cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "Failed to get Kops cluster config: %s", stderrBuf.String())
	}

	log.Debugf("Downloaded config for kops cluster:\n%s", stdoutBuf.String())

	kopsYamlConfig := map[string]interface{}{}
	err = yaml.Unmarshal(stdoutBuf.Bytes(), kopsYamlConfig)
	if err != nil {
		return errors.Wrap(err, "Error parsing kops config")
	}
	log.Debugf("Yaml kopsYamlConfig:\n%s", kopsYamlConfig)

	specs, err := convert.MapInterfaceInterfaceToMapStringInterface(
		provisionerValues["specs"].(map[interface{}]interface{}))
	if err != nil {
		return errors.WithStack(err)
	}

	clusterSpecs := specs["cluster"]

	specValues := map[string]interface{}{"spec": clusterSpecs}

	log.Debugf("Spec to merge in:\n%s", specValues)

	// patch in the configured spec
	mergo.Merge(&kopsYamlConfig, specValues, mergo.WithOverride)

	log.Debugf("Merged config is:\n%s", kopsYamlConfig)

	yamlBytes, err := yaml.Marshal(&kopsYamlConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	yamlString := string(yamlBytes[:])
	log.Debugf("Merged config:\n%s", yamlString)

	// if the merged values are the same as the original, skip replacing the config

	// write the merged data to a temp file because we can't pipe it into kops
	tmpfile, err := ioutil.TempFile("", "kops.*.txt")
	if err != nil {
		log.Fatal(err)
	}

	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write([]byte(yamlString)); err != nil {
		tmpfile.Close()
		log.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		log.Fatal(err)
	}

	// update the cluster

	log.Debugf("Patching kops cluster config...")
	args2 := []string{
		"replace",
		"-f",
		tmpfile.Name(),
	}

	args2 = parameteriseValues(args2, kopsConfig.Params.Global)

	cmd2 := exec.CommandContext(ctx, KOPS_PATH, args2...)
	cmd2.Env = os.Environ()
	cmd2.Stdout = &stdoutBuf
	cmd2.Stderr = &stderrBuf
	err = cmd2.Run()
	if err != nil {
		return errors.Wrapf(err, "Failed to patch Kops cluster config: %s", stderrBuf.String())
	}

	log.Info("Config of Kops cluster patched.")

	log.Debug("Patching instance group configs...")

	igSpecs := specs["instanceGroups"].(map[interface{}]interface{})

	for instanceGroupName, newSpec := range igSpecs {
		specValues := map[string]interface{}{"spec": newSpec}
		p.patchInstanceGroup(kopsConfig, instanceGroupName.(string), specValues)
	}

	log.Debugf("Applying kops cluster config...")
	args3 := []string{
		"update",
		"cluster",
		"--yes",
	}

	args3 = parameteriseValues(args3, kopsConfig.Params.Global)

	log.Infof("Running Kops cluster update... Executing: %s %s", KOPS_PATH,
		strings.Join(args3, " "))

	cmd3 := exec.Command(KOPS_PATH, args3...)
	cmd3.Env = os.Environ()
	cmd3.Stdout = &stdoutBuf
	cmd3.Stderr = &stderrBuf
	err = cmd3.Run()
	if err != nil {
		return errors.Wrapf(err, "Failed to apply Kops cluster config: %s", stderrBuf.String())
	}

	return nil
}

func (p KopsProvisioner) patchInstanceGroup(kopsConfig *KopsConfig, instanceGroupName string,
	newSpec map[string]interface{}) error {
	args := []string{
		"get",
		"instancegroups",
		instanceGroupName,
		"-o",
		"yaml",
	}

	args = parameteriseValues(args, kopsConfig.Params.Global)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel() // The cancel should be deferred so resources are cleaned up

	cmd := exec.CommandContext(ctx, KOPS_PATH, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	log.Debug("Downloading IG config for kops cluster")

	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "Failed to get Kops IG config: %s", stderrBuf.String())
	}

	log.Debugf("Downloaded IG config for kops cluster:\n%s", stdoutBuf.String())

	kopsYamlConfig := map[string]interface{}{}
	err = yaml.Unmarshal(stdoutBuf.Bytes(), kopsYamlConfig)
	if err != nil {
		return errors.Wrap(err, "Error parsing kops IG config")
	}
	log.Debugf("Yaml IG kopsYamlConfig:\n%s", kopsYamlConfig)

	// patch in the configured spec
	mergo.Merge(&kopsYamlConfig, newSpec, mergo.WithOverride)

	log.Debugf("Merged IG config is:\n%s", kopsYamlConfig)

	yamlBytes, err := yaml.Marshal(&kopsYamlConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	yamlString := string(yamlBytes[:])
	log.Debugf("Merged config:\n%s", yamlString)

	// write the merged data to a temp file because we can't pipe it into kops
	tmpfile, err := ioutil.TempFile("", "kops.*.txt")
	if err != nil {
		log.Fatal(err)
	}

	defer os.Remove(tmpfile.Name()) // clean up

	if _, err := tmpfile.Write([]byte(yamlString)); err != nil {
		tmpfile.Close()
		log.Fatal(err)
	}
	if err := tmpfile.Close(); err != nil {
		log.Fatal(err)
	}

	// update the cluster config

	log.Debugf("Patching kops IG config...")
	args2 := []string{
		"replace",
		"-f",
		tmpfile.Name(),
	}

	args2 = parameteriseValues(args2, kopsConfig.Params.Global)

	cmd2 := exec.CommandContext(ctx, KOPS_PATH, args2...)
	cmd2.Env = os.Environ()
	cmd2.Stdout = &stdoutBuf
	cmd2.Stderr = &stderrBuf
	err = cmd2.Run()
	if err != nil {
		return errors.Wrapf(err, "Failed to patch Kops IG config: %s", stderrBuf.String())
	}

	log.Info("IG config of Kops cluster patched.")

	return nil
}

// Converts YAML parameters to CLI args
func parameteriseValues(args []string, valueMap map[string]string) []string {
	for k, v := range valueMap {
		key := strings.Replace(k, "_", "-", -1)
		args = append(args, "--"+key)

		if fmt.Sprintf("%v", v) != "" {
			args = append(args, fmt.Sprintf("%v", v))
		}
	}

	return args
}

// Parses the Kops provisioner config
func getKopsConfig(provisionerValues map[interface{}]interface{}) (*KopsConfig, error) {
	log.Debugf("Marshalling: %#v", provisionerValues)

	// marshal then unmarshal the provisioner values to get the command parameters
	byteData, err := yaml.Marshal(provisionerValues)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	log.Debugf("Marshalled to: %s", string(byteData[:]))

	var kopsConfig KopsConfig
	err = yaml.Unmarshal(byteData, &kopsConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &kopsConfig, nil
}
