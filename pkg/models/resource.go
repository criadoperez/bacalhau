package models

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/BTBurke/k8sresource"
	"github.com/c2h5oh/datasize"
	"github.com/hashicorp/go-multierror"
	"github.com/rs/zerolog/log"
)

type ResourcesConfig struct {
	// CPU https://github.com/BTBurke/k8sresource string
	CPU string `json:"CPU,omitempty"`
	// Memory github.com/c2h5oh/datasize string
	Memory string `json:"Memory,omitempty"`
	// Memory github.com/c2h5oh/datasize string
	Disk string `json:"Disk,omitempty"`
	GPU  string `json:"GPU,omitempty"`
}

// Normalize normalizes the resources
func (r *ResourcesConfig) Normalize() {
	if r == nil {
		return
	}
	sanitizeResourceString := func(s string) string {
		// lower case, allow Mi, Gi to mean Mb, Gb, and remove spaces
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "i", "b")
		s = strings.ReplaceAll(s, " ", "")
		s = strings.ReplaceAll(s, "\n", "")
		return s
	}

	r.CPU = sanitizeResourceString(r.CPU)
	r.Memory = sanitizeResourceString(r.Memory)
	r.Disk = sanitizeResourceString(r.Disk)
	r.GPU = sanitizeResourceString(r.GPU)
}

// Copy returns a deep copy of the resources
func (r *ResourcesConfig) Copy() *ResourcesConfig {
	if r == nil {
		return nil
	}
	newR := new(ResourcesConfig)
	*newR = *r
	return newR
}

// Validate returns an error if the resources are invalid
func (r *ResourcesConfig) Validate() error {
	if r == nil {
		return errors.New("missing resources")
	}
	resources, err := r.ToResources()
	if err != nil {
		return err
	}
	return resources.Validate()
}

// ToResources converts the resources config to resources
func (r *ResourcesConfig) ToResources() (*Resources, error) {
	if r == nil {
		return nil, errors.New("missing resources")
	}
	r.Normalize()
	var mErr multierror.Error
	res := &Resources{}

	if r.CPU != "" {
		cpu, err := k8sresource.NewCPUFromString(r.CPU)
		if err != nil {
			mErr.Errors = append(mErr.Errors, fmt.Errorf("invalid CPU value: %s", r.CPU))
		}
		res.CPU = cpu.ToFloat64()
	}
	if r.Memory != "" {
		mem, err := datasize.ParseString(r.Memory)
		if err != nil {
			mErr.Errors = append(mErr.Errors, fmt.Errorf("invalid memory value: %s", r.Memory))
		}
		res.Memory = mem.Bytes()
	}
	if r.Disk != "" {
		disk, err := datasize.ParseString(r.Disk)
		if err != nil {
			mErr.Errors = append(mErr.Errors, fmt.Errorf("invalid disk value: %s", r.Disk))
		}
		res.Disk = disk.Bytes()
	}
	if r.GPU != "" {
		gpu, err := strconv.ParseUint(r.GPU, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid GPU value: %s", r.GPU)
		}
		res.GPU = gpu
	}

	return res, mErr.ErrorOrNil()
}

type ResourcesConfigBuilder struct {
	resources *ResourcesConfig
}

func NewResourcesConfigBuilder() *ResourcesConfigBuilder {
	return &ResourcesConfigBuilder{resources: &ResourcesConfig{}}
}

func (r *ResourcesConfigBuilder) CPU(cpu string) *ResourcesConfigBuilder {
	r.resources.CPU = cpu
	return r
}

func (r *ResourcesConfigBuilder) Memory(memory string) *ResourcesConfigBuilder {
	r.resources.Memory = memory
	return r
}

func (r *ResourcesConfigBuilder) Disk(disk string) *ResourcesConfigBuilder {
	r.resources.Disk = disk
	return r
}

func (r *ResourcesConfigBuilder) GPU(gpu string) *ResourcesConfigBuilder {
	r.resources.GPU = gpu
	return r
}

func (r *ResourcesConfigBuilder) Build() (*ResourcesConfig, error) {
	r.resources.Normalize()
	return r.resources, r.resources.Validate()
}

// BuildOrDie is the same as Build, but panics if an error occurs
func (r *ResourcesConfigBuilder) BuildOrDie() *ResourcesConfig {
	resources, err := r.Build()
	if err != nil {
		panic(err)
	}
	return resources
}

type Resources struct {
	// CPU units
	CPU float64 `json:"CPU,omitempty"`
	// Memory in bytes
	Memory uint64 `json:"Memory,omitempty"`
	// Disk in bytes
	Disk uint64 `json:"Disk,omitempty"`
	// GPU units
	GPU uint64 `json:"GPU,omitempty"`
}

// Copy returns a deep copy of the resources
func (r *Resources) Copy() *Resources {
	if r == nil {
		return nil
	}
	newR := new(Resources)
	*newR = *r
	return newR
}

// Validate returns an error if the resources are invalid
func (r *Resources) Validate() error {
	if r == nil {
		return errors.New("missing resources")
	}
	var mErr multierror.Error
	if r.CPU < 0 {
		mErr.Errors = append(mErr.Errors, fmt.Errorf("invalid CPU value: %f", r.CPU))
	}
	return mErr.ErrorOrNil()
}

// Merge merges the resources, preferring the current resources
func (r *Resources) Merge(other Resources) *Resources {
	newR := r.Copy()
	if newR.CPU <= 0 {
		newR.CPU = other.CPU
	}
	if newR.Memory <= 0 {
		newR.Memory = other.Memory
	}
	if newR.Disk <= 0 {
		newR.Disk = other.Disk
	}
	if newR.GPU <= 0 {
		newR.GPU = other.GPU
	}
	return newR
}

// Add returns the sum of the resources
func (r *Resources) Add(other Resources) *Resources {
	return &Resources{
		CPU:    r.CPU + other.CPU,
		Memory: r.Memory + other.Memory,
		Disk:   r.Disk + other.Disk,
		GPU:    r.GPU + other.GPU,
	}
}

func (r *Resources) Sub(other Resources) *Resources {
	usage := &Resources{
		CPU:    r.CPU - other.CPU,
		Memory: r.Memory - other.Memory,
		Disk:   r.Disk - other.Disk,
		GPU:    r.GPU - other.GPU,
	}

	if r.LessThan(other) {
		log.Warn().Msgf("Subtracting larger resource usage %s from %s. Replacing negative values with zeros",
			other.String(), r.String())
		if other.CPU > r.CPU {
			usage.CPU = 0
		}
		if other.Memory > r.Memory {
			usage.Memory = 0
		}
		if other.Disk > r.Disk {
			usage.Disk = 0
		}
		if other.GPU > r.GPU {
			usage.GPU = 0
		}
	}

	return usage
}

func (r *Resources) LessThan(other Resources) bool {
	return r.CPU < other.CPU && r.Memory < other.Memory && r.Disk < other.Disk && r.GPU < other.GPU
}

func (r *Resources) LessThanEq(other Resources) bool {
	return r.CPU <= other.CPU && r.Memory <= other.Memory && r.Disk <= other.Disk && r.GPU <= other.GPU
}

func (r *Resources) Max(other Resources) *Resources {
	newR := r.Copy()
	if newR.CPU < other.CPU {
		newR.CPU = other.CPU
	}
	if newR.Memory < other.Memory {
		newR.Memory = other.Memory
	}
	if newR.Disk < other.Disk {
		newR.Disk = other.Disk
	}
	if newR.GPU < other.GPU {
		newR.GPU = other.GPU
	}

	return newR
}

func (r *Resources) IsZero() bool {
	return r.CPU == 0 && r.Memory == 0 && r.Disk == 0 && r.GPU == 0
}

// return string representation of ResourceUsageData
func (r *Resources) String() string {
	mem := datasize.ByteSize(r.Memory)
	disk := datasize.ByteSize(r.Disk)
	return fmt.Sprintf("{CPU: %f, Memory: %s, Disk: %s, GPU: %d}", r.CPU, mem.HR(), disk.HR(), r.GPU)
}

// AllocatedResources is the set of resources to be used by an execution, which
// maybe be resources allocated to a single task or a set of tasks in the future.
type AllocatedResources struct {
	Tasks map[string]*Resources `json:"Tasks"`
}

func (a *AllocatedResources) Copy() *AllocatedResources {
	if a == nil {
		return a
	}
	tasks := make(map[string]*Resources)
	for k, v := range a.Tasks {
		tasks[k] = v.Copy()
	}
	return &AllocatedResources{
		Tasks: tasks,
	}
}

// Total returns the total resources allocated
func (a *AllocatedResources) Total() *Resources {
	if a == nil {
		return nil
	}
	total := &Resources{}
	for _, task := range a.Tasks {
		total = total.Add(*task)
	}
	return total
}
