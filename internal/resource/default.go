package resource

import "time"

// Default returns the registry with every built-in resource.
func Default(now func() time.Time) (*Registry, error) {
	reg := NewRegistry()
	for _, res := range []Resource{
		WithStats(Containers(now)),
		Images(now),
		Volumes(now),
		Networks(now),
		Compose(now),
		DiskUsage(),
		Services(now),
		Tasks(now),
		Nodes(now),
		Stacks(now),
		Secrets(now),
		Configs(now),
	} {
		if err := reg.Register(res); err != nil {
			return nil, err
		}
	}
	return reg, nil
}
