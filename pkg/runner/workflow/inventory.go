package workflow

import "sort"

type HostSpec struct {
	Name    string
	Address string
	Vars    map[string]any
	Groups  []string
}

func (inv Inventory) ResolveHosts() map[string]HostSpec {
	hosts := map[string]HostSpec{}
	groupsByHost := map[string][]string{}

	for name, host := range inv.Hosts {
		addr := host.Address
		if addr == "" {
			addr = name
		}
		hosts[name] = HostSpec{
			Name:    name,
			Address: addr,
			Vars:    copyVars(inv.Vars),
		}
	}

	for groupName, group := range inv.Groups {
		for _, hostName := range group.Hosts {
			groupsByHost[hostName] = append(groupsByHost[hostName], groupName)
			if _, exists := hosts[hostName]; !exists {
				hosts[hostName] = HostSpec{
					Name:    hostName,
					Address: hostName,
					Vars:    copyVars(inv.Vars),
				}
			}
		}
	}

	for hostName, spec := range hosts {
		groupNames := groupsByHost[hostName]
		sort.Strings(groupNames)

		merged := copyVars(inv.Vars)
		for _, groupName := range groupNames {
			if group, ok := inv.Groups[groupName]; ok {
				merged = mergeVars(merged, group.Vars)
			}
		}
		if host, ok := inv.Hosts[hostName]; ok {
			merged = mergeVars(merged, host.Vars)
			if host.Address != "" {
				spec.Address = host.Address
			}
		}

		spec.Vars = merged
		spec.Groups = groupNames
		hosts[hostName] = spec
	}

	return hosts
}

func copyVars(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}

func mergeVars(base, overlay map[string]any) map[string]any {
	out := copyVars(base)
	for k, v := range overlay {
		out[k] = v
	}
	return out
}
