package monarch

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// node 是保留对象键顺序的 JSON 值。Monarch 的语义依赖键顺序：tokenizer 的第一个
// 状态是默认起始状态，cases 按书写顺序逐个尝试；encoding/json 的 map 会丢失顺序。
type node struct {
	kind  nodeKind
	str   string // kString；kRegex 时为源码
	flags string // kRegex
	num   float64
	b     bool
	arr   []*node
	keys  []string
	vals  []*node
}

type nodeKind int

const (
	kNull nodeKind = iota
	kBool
	kNumber
	kString
	kArray
	kObject
	kRegex // {"$regex": source, "flags": flags}
)

func (n *node) get(key string) *node {
	if n == nil || n.kind != kObject {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

func (n *node) isString() bool { return n != nil && n.kind == kString }

// truthy 按 JS 的真值规则判断（未定义视为假）。
func (n *node) truthy() bool {
	if n == nil {
		return false
	}
	switch n.kind {
	case kNull:
		return false
	case kBool:
		return n.b
	case kNumber:
		return n.num != 0
	case kString:
		return n.str != ""
	}
	return true
}

func parseJSON(b []byte) (*node, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	n, err := parseValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("monarch: trailing data after grammar JSON")
	}
	return n, nil
}

func parseValue(dec *json.Decoder) (*node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch v := tok.(type) {
	case nil:
		return &node{kind: kNull}, nil
	case bool:
		return &node{kind: kBool, b: v}, nil
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return nil, err
		}
		return &node{kind: kNumber, num: f}, nil
	case string:
		return &node{kind: kString, str: v}, nil
	case json.Delim:
		switch v {
		case '[':
			n := &node{kind: kArray}
			for dec.More() {
				e, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				n.arr = append(n.arr, e)
			}
			_, err := dec.Token()
			return n, err
		case '{':
			n := &node{kind: kObject}
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k := kt.(string)
				val, err := parseValue(dec)
				if err != nil {
					return nil, err
				}
				replaced := false
				for i, ek := range n.keys { // 重复键：后者覆盖值，位置不变（同 JS）
					if ek == k {
						n.vals[i], replaced = val, true
						break
					}
				}
				if !replaced {
					n.keys = append(n.keys, k)
					n.vals = append(n.vals, val)
				}
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			if src := n.get("$regex"); src.isString() {
				re := &node{kind: kRegex, str: src.str}
				if f := n.get("flags"); f.isString() {
					re.flags = f.str
				}
				return re, nil
			}
			return n, nil
		}
	}
	return nil, fmt.Errorf("monarch: unexpected JSON token %v", tok)
}
