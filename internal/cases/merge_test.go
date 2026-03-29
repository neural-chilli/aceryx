package cases

import "testing"

func TestDeepMerge(t *testing.T) {
	base := map[string]interface{}{
		"obj": map[string]interface{}{"a": 1.0, "b": 2.0},
		"arr": []interface{}{1.0, 2.0},
		"x":   "old",
	}
	patch := map[string]interface{}{
		"obj": map[string]interface{}{"b": 3.0, "c": 4.0},
		"arr": []interface{}{9.0},
		"x":   nil,
	}
	merged := DeepMerge(base, patch)

	obj := merged["obj"].(map[string]interface{})
	if obj["a"].(float64) != 1 || obj["b"].(float64) != 3 || obj["c"].(float64) != 4 {
		t.Fatalf("unexpected object merge: %+v", obj)
	}
	arr := merged["arr"].([]interface{})
	if len(arr) != 1 || arr[0].(float64) != 9 {
		t.Fatalf("expected array replacement, got %+v", arr)
	}
	if merged["x"] != nil {
		t.Fatalf("expected null handling, got %+v", merged["x"])
	}
}
