package kapp

import (
	"github.com/pkg/errors"
	"github.com/sugarkube/sugarkube/internal/pkg/acquirer"
	"github.com/sugarkube/sugarkube/internal/pkg/convert"
	"github.com/sugarkube/sugarkube/internal/pkg/log"
	"github.com/sugarkube/sugarkube/internal/pkg/vars"
	"gopkg.in/yaml.v2"
)

type installerConfig struct {
	kapp         string
	searchValues []string
	params       map[string]string
}

type Kapp struct {
	id              string
	shouldBePresent bool // if true, this kapp should be present
	// after completing, otherwise it should
	// be absent
	installerConfig installerConfig
	sources         []acquirer.Acquirer
}

const PRESENT_KEY = "present"
const ABSENT_KEY = "absent"
const SOURCES_KEY = "sources"

// Parses a manifest file and returns a list of kapps on success
func parseManifest(manifestPath string) ([]Kapp, error) {
	log.Debugf("Parsing manifest: %s", manifestPath)

	kapps := make([]Kapp, 0)

	data, err := vars.LoadYamlFile(manifestPath)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	log.Debugf("Loaded manifest data: %#v", data)

	presentKapps := data[PRESENT_KEY]

	// parse each kapp definition
	for k, v := range presentKapps.(map[interface{}]interface{}) {
		kapp := Kapp{
			id:              k.(string),
			shouldBePresent: true,
		}

		log.Debugf("kapp=%s, v=%#v", kapp, v)

		// parse the list of sources
		valuesMap, err := convert.MapInterfaceInterfaceToMapStringInterface(v.(map[interface{}]interface{}))
		if err != nil {
			return nil, errors.Wrapf(err, "Error converting manifest value to map")
		}

		// marshal and unmarshal the list of sources
		sourcesBytes, err := yaml.Marshal(valuesMap[SOURCES_KEY])
		if err != nil {
			return nil, errors.Wrapf(err, "Error marshalling sources yaml: %#v", v)
		}

		log.Debugf("Marshalled sources YAML: %s", sourcesBytes)

		sourcesMaps := []map[interface{}]interface{}{}
		err = yaml.UnmarshalStrict(sourcesBytes, &sourcesMaps)
		if err != nil {
			return nil, errors.Wrapf(err, "Error unmarshalling yaml: %s", sourcesBytes)
		}

		log.Debugf("sourcesMaps=%#v", sourcesMaps)

		acquirers := make([]acquirer.Acquirer, 0)
		// now we have a list of sources, get the acquirer for each one
		for _, sourceMap := range sourcesMaps {
			sourceStringMap, err := convert.MapInterfaceInterfaceToMapStringString(sourceMap)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			acquirerImpl, err := acquirer.NewAcquirer(sourceStringMap)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			log.Debugf("Got acquirer %#v", acquirerImpl)

			acquirers = append(acquirers, acquirerImpl)
		}

		kapp.sources = acquirers

		log.Debugf("Parsed kapp=%#v", kapp)

		kapps = append(kapps, kapp)
	}

	log.Debugf("Parsed kapps to install: %#v", kapps)

	// todo - implement
	//absentKapps := data[ABSENT_KEY]

	return kapps, nil
}

// Parses manifest files and returns a list of kapps on success
func ParseManifests(manifests []string) ([]Kapp, error) {
	log.Debugf("Parsing %d manifest(s)", len(manifests))

	kapps := make([]Kapp, 0)

	for _, manifest := range manifests {
		manifestKapps, err := parseManifest(manifest)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		kapps = append(kapps, manifestKapps...)
	}

	return kapps, nil
}
