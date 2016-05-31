package crane

import (
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"
)

// Create a map of stubbed containers out of the provided set
func NewStubbedContainerMap(exists bool, containers ...Container) ContainerMap {
	containerMap := make(map[string]Container)
	for _, container := range containers {
		containerMap[container.Name()] = &StubbedContainer{container, exists}
	}
	return containerMap
}

type StubbedContainer struct {
	Container
	exists bool
}

func (stubbedContainer *StubbedContainer) Exists() bool {
	return stubbedContainer.exists
}

func TestConfigFilenames(t *testing.T) {
	// With given fileName
	fileName := "some/file.yml"
	files := configFilenames(fileName)
	assert.Equal(t, []string{fileName}, files)
	// Without given fileName
	files = configFilenames("")
	assert.Equal(t, []string{"crane.json", "crane.yaml", "crane.yml"}, files)
}

func TestFindConfig(t *testing.T) {
	f, _ := ioutil.TempFile("", "crane.yml")
	defer syscall.Unlink(f.Name())
	configName := filepath.Base(f.Name())
	absConfigName := os.TempDir() + "/" + configName
	var fileName string

	// Finds config in current dir
	os.Chdir(os.TempDir())
	fileName = findConfig(configName)
	assert.Equal(t, f.Name(), fileName)

	// Finds config in parent dir
	d, _ := ioutil.TempDir("", "sub")
	defer syscall.Unlink(d)
	os.Chdir(d)
	fileName = findConfig(configName)
	assert.Equal(t, f.Name(), fileName)

	// Finds config with absolute path
	fileName = findConfig(absConfigName)
	assert.Equal(t, f.Name(), fileName)
}

func TestUnmarshal(t *testing.T) {
	var actual *config
	json := []byte(
		`{
    "containers": {
        "apache": {
            "dockerfile": "apache",
            "image": "michaelsauter/apache",
            "run": {
                "volumes-from": ["crane_app"],
                "publish": ["80:80"],
                "env": {
                    "foo": 1234,
                    "4567": "bar",
                    "true": false
                },
                "label": [
                    "foo=1234",
                    "4567=bar",
                    "true=false"
                ],
                "link": ["crane_mysql:db", "crane_memcached:cache"],
                "detach": true
            }
        }
    },
    "groups": {
        "default": [
            "apache"
        ]
    },
    "hooks": {
        "apache": {
            "post-stop": "echo apache container stopped!\n"
        },
        "default": {
            "pre-start": "echo start...",
            "post-start": "echo start done!\n"
        }
    },
    "networks": {
        "foo": {}
    },
    "volumes": {
        "bar": {}
    }
}
`)
	actual = unmarshal(json, ".json")
	assert.Len(t, actual.RawContainers, 1)
	assert.Len(t, actual.RawContainers["apache"].RunParams().Env(), 3)
	assert.Len(t, actual.RawContainers["apache"].RunParams().Label(), 3)
	assert.Len(t, actual.RawContainers["apache"].RunParams().Link(), 2)
	assert.Len(t, actual.RawGroups, 1)
	assert.Len(t, actual.RawHooks, 2)
	assert.Len(t, actual.RawNetworks, 1)
	assert.Len(t, actual.RawVolumes, 1)
	assert.NotEmpty(t, actual.RawHooks["default"].RawPreStart)
	assert.NotEmpty(t, actual.RawHooks["default"].RawPostStart)

	yaml := []byte(
		`containers:
  apache:
    dockerfile: apache
    image: michaelsauter/apache
    run:
      volumes-from: ["crane_app"]
      publish: ["80:80"]
      env:
        foo: 1234
        4567: bar
        true: false
      label:
        - foo=1234
        - 4567=bar
        - true=false
      link: ["crane_mysql:db", "crane_memcached:cache"]
      detach: true
groups:
  default:
    - apache
hooks:
  apache:
    post-stop: echo apache container stopped!\n
  default:
    pre-start: echo start...
    post-start: echo start done!
networks:
  foo: {}
volumes:
  bar: {}
`)
	actual = unmarshal(yaml, ".yml")
	assert.Len(t, actual.RawContainers, 1)
	assert.Len(t, actual.RawContainers["apache"].RunParams().Env(), 3)
	assert.Len(t, actual.RawContainers["apache"].RunParams().Label(), 3)
	assert.Len(t, actual.RawContainers["apache"].RunParams().Link(), 2)
	assert.Len(t, actual.RawGroups, 1)
	assert.Len(t, actual.RawHooks, 2)
	assert.Len(t, actual.RawNetworks, 1)
	assert.Len(t, actual.RawVolumes, 1)
	assert.NotEmpty(t, actual.RawHooks["default"].RawPreStart)
	assert.NotEmpty(t, actual.RawHooks["default"].RawPostStart)
}

func TestUnmarshalInvalidJSON(t *testing.T) {
	json := []byte(
		`{
    "containers": {
        "apache": {
            "image": "michaelsauter/apache",
            "run": {
                "publish": "shouldbeanarray"
            }
        }
    }
}
`)
	assert.Panics(t, func() {
		unmarshal(json, ".json")
	})
}

func TestUnmarshalInvalidYAML(t *testing.T) {
	yaml := []byte(
		`containers:
  apache:
    image: michaelsauter/apache
    run:
      publish: "shouldbeanarray"
`)
	assert.Panics(t, func() {
		unmarshal(yaml, ".yml")
	})
}

func TestUnmarshalEmptyNetworkOrVolume(t *testing.T) {
	yaml := []byte(
		`networks:
  foo:
volumes:
  bar:
`)
	config := unmarshal(yaml, ".yml")
	config.setNetworkMap()
	config.setVolumeMap()
	assert.Equal(t, "foo", config.networkMap["foo"].Name())
	assert.Equal(t, "bar", config.volumeMap["bar"].Name())
}

func TestInitialize(t *testing.T) {
	// use different, undefined environment variables throughout the config to detect any issue in expansion
	rawContainerMap := map[string]*container{
		"${UNDEFINED1}a": &container{},
		"${UNDEFINED2}b": &container{},
	}
	rawGroups := map[string][]string{
		"${UNDEFINED3}default": []string{
			"${UNDEFINED4}a",
			"${UNDEFINED4}b",
		},
	}
	rawHooksMap := map[string]hooks{
		"${UNDEFINED5}default": hooks{
			RawPreStart:  "${UNDEFINED6}default-pre-start",
			RawPostStart: "${UNDEFINED7}default-post-start",
		},
		"${UNDEFINED8}a": hooks{
			RawPreStart: "${UNDEFINED9}custom-pre-start",
		},
	}
	c := &config{
		RawContainers: rawContainerMap,
		RawGroups:     rawGroups,
		RawHooks:      rawHooksMap,
	}
	c.initialize()
	assert.Equal(t, "a", c.containerMap["a"].Name())
	assert.Equal(t, "b", c.containerMap["b"].Name())
	assert.Equal(t, map[string][]string{"default": []string{"a", "b"}}, c.groups)
	assert.Equal(t, "custom-pre-start", c.containerMap["a"].Hooks().PreStart(), "Container should have a custom pre-start hook overriding the default one")
	assert.Equal(t, "default-post-start", c.containerMap["a"].Hooks().PostStart(), "Container should have a default post-start hook")
	assert.Equal(t, "default-pre-start", c.containerMap["b"].Hooks().PreStart(), "Container should have a default post-start hook")
	assert.Equal(t, "default-post-start", c.containerMap["b"].Hooks().PostStart(), "Container should have a default post-start hook")
}

func TestInitializeAmbiguousHooks(t *testing.T) {
	rawContainerMap := map[string]*container{
		"a": &container{},
		"b": &container{},
	}
	rawGroups := map[string][]string{
		"group1": []string{"a"},
		"group2": []string{"a", "b"},
	}
	rawHooksMap := map[string]hooks{
		"group1": hooks{RawPreStart: "group1-pre-start"},
		"group2": hooks{RawPreStart: "group2-pre-start"},
	}
	c := &config{
		RawContainers: rawContainerMap,
		RawGroups:     rawGroups,
		RawHooks:      rawHooksMap,
	}
	assert.Panics(t, func() {
		c.initialize()
	})
}

func TestValidate(t *testing.T) {
	rawContainerMap := map[string]*container{
		"a": &container{RawName: "a", RawImage: "ubuntu"},
		"b": &container{RawName: "b", RawImage: "ubuntu"},
	}
	c := &config{RawContainers: rawContainerMap}
	assert.NotPanics(t, func() {
		c.validate()
	})
	rawContainerMap = map[string]*container{
		"a": &container{RawName: "a", RawImage: "ubuntu"},
		"b": &container{RawName: "b"},
	}
	c = &config{RawContainers: rawContainerMap}
	assert.Panics(t, func() {
		c.validate()
	})
}

func TestDependencyMap(t *testing.T) {
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a", RawRun: RunParameters{RawLink: []string{"b:b"}}},
		&container{RawName: "b", RawRun: RunParameters{RawLink: []string{"c:c"}}},
		&container{RawName: "c"},
	)
	c := &config{containerMap: containerMap}

	dependencyMap := c.DependencyMap([]string{})
	assert.Len(t, dependencyMap, 3)
	// make sure a new map is returned each time
	delete(dependencyMap, "a")
	assert.Len(t, c.DependencyMap([]string{}), 3)

	dependencyMap = c.DependencyMap([]string{"b"})
	assert.Len(t, dependencyMap, 2)
}

func TestContainersForReference(t *testing.T) {
	var containers []string
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a"},
		&container{RawName: "b"},
		&container{RawName: "c"},
	)

	// No target given
	// If default group exist, it returns its containers
	groups := map[string][]string{
		"default": []string{"a", "b"},
	}
	c := &config{containerMap: containerMap, groups: groups}
	containers = c.ContainersForReference("")
	assert.Equal(t, []string{"a", "b"}, containers)
	// If no default group, returns all containers
	c = &config{containerMap: containerMap}
	containers = c.ContainersForReference("")
	sort.Strings(containers)
	assert.Equal(t, []string{"a", "b", "c"}, containers)
	// Target given
	// Target is a group
	groups = map[string][]string{
		"second": []string{"b", "c"},
	}
	c = &config{containerMap: containerMap, groups: groups}
	containers = c.ContainersForReference("second")
	assert.Equal(t, []string{"b", "c"}, containers)
	// Target is a container
	containers = c.ContainersForReference("a")
	assert.Equal(t, []string{"a"}, containers)
}

func TestContainersForReferenceInvalidReference(t *testing.T) {
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a"},
		&container{RawName: "b"},
	)
	groups := map[string][]string{
		"foo": []string{"a", "doesntexist", "b"},
	}
	c := &config{containerMap: containerMap, groups: groups}
	assert.Panics(t, func() {
		c.ContainersForReference("foo")
	})
	assert.Panics(t, func() {
		c.ContainersForReference("doesntexist")
	})
}

func TestContainersForReferenceDeduplication(t *testing.T) {
	containerMap := NewStubbedContainerMap(true,
		&container{RawName: "a"},
		&container{RawName: "b"},
	)
	groups := map[string][]string{
		"foo": []string{"a", "b", "a"},
	}
	c := &config{containerMap: containerMap, groups: groups}
	containers := c.ContainersForReference("foo")
	assert.Equal(t, []string{"a", "b"}, containers)
}

func TestNetworkNames(t *testing.T) {
	var networks []string
	var networkMap map[string]Network
	var c Config

	networkMap = map[string]Network{}
	c = &config{networkMap: networkMap}
	networks = c.NetworkNames()
	assert.Equal(t, []string{}, networks)

	networkMap = map[string]Network{
		"foo": &network{},
		"bar": &network{},
	}
	c = &config{networkMap: networkMap}
	networks = c.NetworkNames()
	assert.Equal(t, []string{"bar", "foo"}, networks)
}

func TestVolumeNames(t *testing.T) {
	var volumes []string
	var volumeMap map[string]Volume
	var c Config

	volumeMap = map[string]Volume{}
	c = &config{volumeMap: volumeMap}
	volumes = c.VolumeNames()
	assert.Equal(t, []string{}, volumes)

	volumeMap = map[string]Volume{
		"foo": &network{},
		"bar": &network{},
	}
	c = &config{volumeMap: volumeMap}
	volumes = c.VolumeNames()
	assert.Equal(t, []string{"bar", "foo"}, volumes)
}

func TestConfigNetwork(t *testing.T) {
	var networkMap map[string]*network
	var c *config

	networkMap = map[string]*network{}
	c = &config{RawNetworks: networkMap}
	c.setNetworkMap()
	assert.Equal(t, nil, c.Network("foo"))

	networkMap = map[string]*network{
		"foo": &network{},
		"bar": &network{},
	}
	c = &config{RawNetworks: networkMap}
	c.setNetworkMap()
	assert.Equal(t, "bar", c.Network("bar").Name())
}

func TestConfigVolume(t *testing.T) {
	var rawVolumes map[string]*volume
	var c *config

	rawVolumes = map[string]*volume{}
	c = &config{RawVolumes: rawVolumes}
	c.setVolumeMap()
	assert.Equal(t, nil, c.Volume("foo"))

	rawVolumes = map[string]*volume{
		"foo": &volume{},
		"bar": &volume{},
	}
	c = &config{RawVolumes: rawVolumes}
	c.setVolumeMap()
	assert.Equal(t, "bar", c.Volume("bar").Name())
}
