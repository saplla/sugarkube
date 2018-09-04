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

package cluster

import (
	"fmt"
	"github.com/imdario/mergo"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/sugarkube/sugarkube/internal/pkg/cmd"
	"github.com/sugarkube/sugarkube/internal/pkg/kapp"
	"github.com/sugarkube/sugarkube/internal/pkg/log"
	"github.com/sugarkube/sugarkube/internal/pkg/provider"
	"github.com/sugarkube/sugarkube/internal/pkg/provisioner"
	"io"
)

// Launches a cluster, either local or remote.

type createCmd struct {
	out           io.Writer
	dryRun        bool
	stackName     string
	stackFile     string
	provider      string
	provisioner   string
	varsFilesDirs cmd.Files
	profile       string
	account       string
	cluster       string
	region        string
	manifests     cmd.Files
	onlineTimeout uint32
	readyTimeout  uint32
}

func newCreateCmd(out io.Writer) *cobra.Command {

	c := &createCmd{
		out: out,
	}

	cmd := &cobra.Command{
		Use:   "create [flags]",
		Short: fmt.Sprintf("Create a cluster"),
		Long: `Create a new cluster, either local or remote.

If creating a named stack, just pass the stack name and path to the config file 
it's defined in, e.g.

	$ sugarkube cluster create --stack-name dev1 --stack-config /path/to/stacks.yaml

Otherwise specify the provider, profile, etc. on the command line, or to override
values in a stack config file. CLI args take precedence over values in stack 
config files.

Note: Not all providers require all arguments. See documentation for help.
`,
		RunE: c.run,
	}

	f := cmd.Flags()
	f.BoolVar(&c.dryRun, "dry-run", false, "show what would happen but don't create a cluster")
	f.StringVarP(&c.stackName, "stack-name", "n", "", "name of a stack to launch (required when passing --stack-config)")
	f.StringVarP(&c.stackFile, "stack-config", "s", "", "path to file defining stacks by name")
	f.StringVarP(&c.provider, "provider", "p", "", "name of provider, e.g. aws, local, etc.")
	f.StringVarP(&c.provisioner, "provisioner", "v", "", "name of provisioner, e.g. kops, minikube, etc.")
	f.StringVarP(&c.profile, "profile", "l", "", "launch profile, e.g. dev, test, prod, etc.")
	f.StringVarP(&c.cluster, "cluster", "c", "", "name of cluster to launch, e.g. dev1, dev2, etc.")
	f.StringVarP(&c.account, "account", "a", "", "string identifier for the account to launch in (for providers that support it)")
	f.StringVarP(&c.region, "region", "r", "", "name of region (for providers that support it)")
	f.VarP(&c.varsFilesDirs, "vars-file-or-dir", "f", "YAML vars file or directory to load (can specify multiple)")
	f.VarP(&c.manifests, "manifest", "m", "YAML manifest file to load (can specify multiple)")
	f.Uint32Var(&c.onlineTimeout, "online-timeout", 600, "max number of seconds to wait for the cluster to come online")
	f.Uint32Var(&c.readyTimeout, "ready-timeout", 600, "max number of seconds to wait for the cluster to become ready")
	return cmd
}

func (c *createCmd) run(cmd *cobra.Command, args []string) error {

	stackConfig, err := ParseStackCliArgs(c.stackName, c.stackFile)
	if err != nil {
		return errors.WithStack(err)
	}

	cliManifests, err := kapp.ParseManifests(c.manifests)
	if err != nil {
		return errors.WithStack(err)
	}

	// CLI args override configured args, so merge them in
	cliStackConfig := &kapp.StackConfig{
		Provider:      c.provider,
		Provisioner:   c.provisioner,
		Profile:       c.profile,
		Cluster:       c.cluster,
		VarsFilesDirs: c.varsFilesDirs,
		Manifests:     cliManifests,
		ReadyTimeout:  c.readyTimeout,
		OnlineTimeout: c.onlineTimeout,
	}

	mergo.Merge(stackConfig, cliStackConfig, mergo.WithOverride)

	log.Debugf("Final stack config: %#v", stackConfig)

	providerImpl, err := provider.NewProvider(stackConfig)
	if err != nil {
		return errors.WithStack(err)
	}

	provisionerImpl, err := provisioner.NewProvisioner(stackConfig.Provisioner)
	if err != nil {
		return errors.WithStack(err)
	}

	online, err := provisioner.IsAlreadyOnline(provisionerImpl, stackConfig, providerImpl)
	if err != nil {
		return errors.WithStack(err)
	}

	if online && !c.dryRun {
		log.Infof("Target cluster is already online. Aborting.")
		return nil
	}

	err = provisioner.Create(provisionerImpl, stackConfig, providerImpl, c.dryRun)
	if err != nil {
		return errors.WithStack(err)
	}

	if c.dryRun {
		log.Infof("Dry run. Skipping cluster readiness check.")
	} else {
		err = provisioner.WaitForClusterReadiness(provisionerImpl, stackConfig, providerImpl)
		if err != nil {
			return errors.WithStack(err)
		}

		log.Infof("Cluster '%s' is ready for use.", stackConfig.Cluster)
	}

	return nil
}

// Reads CLI args for a stack file, name and manifests and returns a pointer
// to a StackConfig or an error.
func ParseStackCliArgs(stackName string, stackFile string) (*kapp.StackConfig, error) {
	stackConfig := &kapp.StackConfig{}
	var err error

	// make sure both stack name and stack file are supplied if either are supplied
	if stackName != "" || stackFile != "" {
		if stackName == "" {
			return nil, errors.New("A stack name is required when supplying the path to a stack config file.")
		}

		if stackFile != "" {
			stackConfig, err = kapp.LoadStackConfig(stackName, stackFile)
			if err != nil {
				return nil, errors.WithStack(err)
			}
		}
	}

	log.Debugf("Parsed stack CLI args to stack config: %#v", stackConfig)

	return stackConfig, nil
}
