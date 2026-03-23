// Package config handles settings parsing and persistence.
package config

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

// updateJSONWithComments applies updates to a comment-preserving JSON document.
// It preserves comments and formatting similarly to the upstream CLI.
func updateJSONWithComments(original []byte, updates Settings) ([]byte, error) {
	value, err := hujson.Parse(original)
	if err != nil {
		return nil, err
	}
	current, err := parseJSONWithComments(original)
	if err != nil {
		return nil, err
	}

	ops := diffJSON(current, updates)
	if len(ops) == 0 {
		return original, nil
	}

	patchOps := make([]patchOp, 0, len(ops))
	for _, op := range ops {
		if op.Op == "remove" {
			if err := removeByPointer(&value, op.Path); err != nil {
				return nil, err
			}
			continue
		}
		patchOps = append(patchOps, op)
	}

	if len(patchOps) > 0 {
		patchBytes, err := json.Marshal(patchOps)
		if err != nil {
			return nil, err
		}
		if err := value.Patch(patchBytes); err != nil {
			return nil, err
		}
	}
	value.Format()
	return value.Pack(), nil
}

type patchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

func diffJSON(current Settings, desired Settings) []patchOp {
	ops := []patchOp{}
	diffValue("", current, desired, &ops)
	return ops
}

// diffValue records JSON patch operations required to transform current into desired.
func diffValue(path string, current, desired any, ops *[]patchOp) {
	if valuesEqual(current, desired) {
		return
	}
	currMap, currOK := asMap(current)
	desMap, desOK := asMap(desired)
	if currOK && desOK {
		removeKeys := keysNotIn(currMap, desMap)
		sort.Strings(removeKeys)
		for _, key := range removeKeys {
			*ops = append(*ops, patchOp{Op: "remove", Path: joinPointer(path, key)})
		}

		keys := mapKeys(desMap)
		sort.Strings(keys)
		for _, key := range keys {
			currVal, hasCurr := currMap[key]
			desVal := desMap[key]
			if !hasCurr {
				*ops = append(*ops, patchOp{Op: "add", Path: joinPointer(path, key), Value: desVal})
				continue
			}
			diffValue(joinPointer(path, key), currVal, desVal, ops)
		}
		return
	}

	currArr, currArrOK := asSlice(current)
	desArr, desArrOK := asSlice(desired)
	if currArrOK && desArrOK {
		if !slicesEqual(currArr, desArr) {
			*ops = append(*ops, patchOp{Op: "replace", Path: pointerOrRoot(path), Value: desired})
		}
		return
	}

	*ops = append(*ops, patchOp{Op: "replace", Path: pointerOrRoot(path), Value: desired})
}

func pointerOrRoot(path string) string {
	if path == "" {
		return ""
	}
	return path
}

// joinPointer builds a JSON pointer path segment.
func joinPointer(path string, key string) string {
	escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
	if path == "" {
		return "/" + escaped
	}
	return path + "/" + escaped
}

func keysNotIn(src map[string]any, want map[string]any) []string {
	out := []string{}
	for key := range src {
		if _, ok := want[key]; !ok {
			out = append(out, key)
		}
	}
	return out
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func asMap(v any) (map[string]any, bool) {
	switch typed := v.(type) {
	case Settings:
		return map[string]any(typed), true
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func asSlice(v any) ([]any, bool) {
	switch typed := v.(type) {
	case []any:
		return typed, true
	default:
		return nil, false
	}
}

// valuesEqual compares JSON-like values, normalizing numeric types.
func valuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == b
	}
	ma, okA := asMap(a)
	mb, okB := asMap(b)
	if okA && okB {
		if len(ma) != len(mb) {
			return false
		}
		for k, va := range ma {
			vb, ok := mb[k]
			if !ok {
				return false
			}
			if !valuesEqual(va, vb) {
				return false
			}
		}
		return true
	}

	sa, okA := asSlice(a)
	sb, okB := asSlice(b)
	if okA && okB {
		return slicesEqual(sa, sb)
	}

	if nsA, ok := numberString(a); ok {
		if nsB, ok := numberString(b); ok {
			return nsA == nsB
		}
	}

	return reflect.DeepEqual(a, b)
}

func slicesEqual(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !valuesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

func numberString(v any) (string, bool) {
	switch n := v.(type) {
	case json.Number:
		return n.String(), true
	case float64:
		return strconv.FormatFloat(n, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(n), 'f', -1, 32), true
	case int:
		return strconv.Itoa(n), true
	case int64:
		return strconv.FormatInt(n, 10), true
	case int32:
		return strconv.FormatInt(int64(n), 10), true
	default:
		return "", false
	}
}

func removeByPointer(root *hujson.Value, pointer string) error {
	if pointer == "" {
		return errors.New("cannot remove root value")
	}
	parts, err := splitPointer(pointer)
	if err != nil {
		return err
	}
	if len(parts) == 0 {
		return errors.New("invalid pointer")
	}

	parent := root
	for i := 0; i < len(parts)-1; i++ {
		segment := parts[i]
		switch node := parent.Value.(type) {
		case *hujson.Object:
			member := findObjectMember(node, segment)
			if member == nil {
				return nil
			}
			parent = &member.Value
		case *hujson.Array:
			index, err := parseIndex(segment)
			if err != nil || index < 0 || index >= len(node.Elements) {
				return nil
			}
			parent = &node.Elements[index]
		default:
			return nil
		}
	}

	last := parts[len(parts)-1]
	switch node := parent.Value.(type) {
	case *hujson.Object:
		removeObjectMember(node, last)
	case *hujson.Array:
		index, err := parseIndex(last)
		if err != nil || index < 0 || index >= len(node.Elements) {
			return nil
		}
		removeArrayElement(node, index)
	}
	return nil
}

func splitPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errors.New("invalid JSON pointer")
	}
	parts := strings.Split(pointer[1:], "/")
	for i := range parts {
		parts[i] = strings.ReplaceAll(strings.ReplaceAll(parts[i], "~1", "/"), "~0", "~")
	}
	return parts, nil
}

func parseIndex(segment string) (int, error) {
	if segment == "" {
		return 0, errors.New("empty index")
	}
	return strconv.Atoi(segment)
}

func findObjectMember(obj *hujson.Object, name string) *hujson.ObjectMember {
	for i := range obj.Members {
		member := &obj.Members[i]
		if literal, ok := member.Name.Value.(hujson.Literal); ok {
			if literal.String() == name {
				return member
			}
		}
	}
	return nil
}

func removeObjectMember(obj *hujson.Object, name string) {
	for i := range obj.Members {
		member := obj.Members[i]
		if literal, ok := member.Name.Value.(hujson.Literal); ok {
			if literal.String() != name {
				continue
			}
		} else {
			continue
		}

		extras := append([]byte{}, member.Name.BeforeExtra...)
		extras = append(extras, member.Name.AfterExtra...)
		extras = append(extras, member.Value.BeforeExtra...)
		extras = append(extras, member.Value.AfterExtra...)
		if len(extras) > 0 {
			obj.AfterExtra = append(extras, obj.AfterExtra...)
		}

		copy(obj.Members[i:], obj.Members[i+1:])
		obj.Members = obj.Members[:len(obj.Members)-1]
		return
	}
}

func removeArrayElement(arr *hujson.Array, index int) {
	element := arr.Elements[index]
	extras := append([]byte{}, element.BeforeExtra...)
	extras = append(extras, element.AfterExtra...)
	if len(extras) > 0 {
		arr.AfterExtra = append(extras, arr.AfterExtra...)
	}
	copy(arr.Elements[index:], arr.Elements[index+1:])
	arr.Elements = arr.Elements[:len(arr.Elements)-1]
}
