package cases

import "reflect"

func DeepMerge(base, patch map[string]interface{}) map[string]interface{} {
	if base == nil {
		base = map[string]interface{}{}
	}
	out := cloneMap(base)
	for k, pv := range patch {
		if pv == nil {
			out[k] = nil
			continue
		}

		if pm, ok := pv.(map[string]interface{}); ok {
			if existing, exok := out[k].(map[string]interface{}); exok {
				out[k] = DeepMerge(existing, pm)
			} else {
				out[k] = DeepMerge(map[string]interface{}{}, pm)
			}
			continue
		}

		if rv := reflect.ValueOf(pv); rv.IsValid() && (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) {
			out[k] = pv
			continue
		}

		out[k] = pv
	}
	return out
}

func cloneMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		if m, ok := v.(map[string]interface{}); ok {
			out[k] = cloneMap(m)
			continue
		}
		out[k] = v
	}
	return out
}

func flattenMap(prefix string, value interface{}, out map[string]interface{}) {
	if m, ok := value.(map[string]interface{}); ok {
		for k, v := range m {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			flattenMap(next, v, out)
		}
		return
	}
	out[prefix] = value
}

func ComputeFieldDiff(before, after map[string]interface{}) map[string]FieldDiff {
	lb := map[string]interface{}{}
	la := map[string]interface{}{}
	flattenMap("", before, lb)
	flattenMap("", after, la)

	all := map[string]bool{}
	for k := range lb {
		all[k] = true
	}
	for k := range la {
		all[k] = true
	}

	diff := map[string]FieldDiff{}
	for path := range all {
		bv, bok := lb[path]
		av, aok := la[path]
		if !bok {
			diff[path] = FieldDiff{After: av}
			continue
		}
		if !aok {
			diff[path] = FieldDiff{Before: bv}
			continue
		}
		if !reflect.DeepEqual(bv, av) {
			diff[path] = FieldDiff{Before: bv, After: av}
		}
	}

	delete(diff, "")
	return diff
}
