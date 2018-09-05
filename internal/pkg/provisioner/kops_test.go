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
	"github.com/imdario/mergo"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	"testing"
)

const sampleKopsConfig = `
apiVersion: kops/v1alpha2
kind: Cluster
metadata:
  creationTimestamp: 2018-09-05T09:10:52Z
  name: dev1.eu-west-1.example.com
spec:
  api:
    loadBalancer:
      type: Public
  authorization:
    rbac: {}
  cloudProvider: aws
  etcdClusters:
  - etcdMembers:
    - instanceGroup: master-eu-west-1a
      name: a
    name: main
`

const specToMerge = `
spec:                             # The kops config will be downloaded and this
  docker:                         # will be patched into the spec before
    logOpt:                       # This allows kops clusters to be fully
    - max-size: 10m               # customised and updated automatically.
    logDriver: json-file
  api:
    loadBalancer:
      type: Public
`

const expected = `apiVersion: kops/v1alpha2
kind: Cluster
metadata:
  creationTimestamp: "2018-09-05T09:10:52Z"
  name: dev1.eu-west-1.example.com
spec:
  api:
    loadBalancer:
      type: Public
  authorization:
    rbac: {}
  cloudProvider: aws
  docker:
    logDriver: json-file
    logOpt:
    - max-size: 10m
  etcdClusters:
  - etcdMembers:
    - instanceGroup: master-eu-west-1a
      name: a
    name: main
`

func TestMergeKopsConfig(t *testing.T) {
	kopsConfig := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(sampleKopsConfig), kopsConfig)
	assert.Nil(t, err)
	assert.NotEmpty(t, kopsConfig)

	specData := map[string]interface{}{}
	err = yaml.Unmarshal([]byte(specToMerge), specData)
	assert.Nil(t, err)
	assert.NotEmpty(t, specData)

	// merge specData into kopsConfig
	mergo.Merge(&kopsConfig, specData, mergo.WithOverride)

	yamlBytes, err := yaml.Marshal(&kopsConfig)
	assert.Nil(t, err)

	yamlString := string(yamlBytes[:])

	assert.Equal(t, expected, yamlString)
}
