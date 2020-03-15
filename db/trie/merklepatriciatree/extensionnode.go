// Copyright (c) 2019 IoTeX Foundation
// This is an alpha (internal) release and is not suitable for production. This source code is provided 'as is' and no
// warranties are given as to title or non-infringement, merchantability or fitness for purpose and, to the extent
// permitted by law, all liability for your use of the code is disclaimed. This source code is governed by Apache
// License 2.0 that can be found in the LICENSE file.

package merklepatriciatree

import (
	"github.com/golang/protobuf/proto"

	"github.com/iotexproject/iotex-core/db/trie"
	"github.com/iotexproject/iotex-core/db/trie/triepb"
)

// extensionNode defines a node with a path and point to a child node
type extensionNode struct {
	cacheNode
	path  []byte
	child node
}

func newExtensionNode(
	mpt *merklePatriciaTree,
	path []byte,
	child node,
) (node, error) {
	e := &extensionNode{cacheNode: cacheNode{mpt: mpt}, path: path, child: child}
	e.cacheNode.serializable = e

	return e.store()
}

func newExtensionNodeFromProtoPb(mpt *merklePatriciaTree, pb *triepb.ExtendPb) *extensionNode {
	e := &extensionNode{cacheNode: cacheNode{mpt: mpt}, path: pb.Path, child: newHashNode(mpt, pb.Value)}
	e.cacheNode.serializable = e
	return e
}

func (e *extensionNode) Delete(key keyType, offset uint8) (node, error) {
	trieMtc.WithLabelValues("extensionNode", "delete").Inc()
	matched := e.commonPrefixLength(key[offset:])
	if matched != uint8(len(e.path)) {
		return nil, trie.ErrNotExist
	}
	newChild, err := e.child.Delete(key, offset+matched)
	if err != nil {
		return nil, err
	}
	if newChild == nil {
		return nil, e.delete()
	}
	if hn, ok := newChild.(*hashNode); ok {
		if newChild, err = hn.LoadNode(); err != nil {
			return nil, err
		}
	}
	switch node := newChild.(type) {
	case *extensionNode:
		return node.updatePath(append(e.path, node.path...), false)
	case *branchNode:
		return e.updateChild(node, false)
	default:
		if err := e.delete(); err != nil {
			return nil, err
		}
		return node, nil
	}
}

func (e *extensionNode) Upsert(key keyType, offset uint8, value []byte) (node, error) {
	trieMtc.WithLabelValues("extensionNode", "upsert").Inc()
	matched := e.commonPrefixLength(key[offset:])
	if matched == uint8(len(e.path)) {
		newChild, err := e.child.Upsert(key, offset+matched, value)
		if err != nil {
			return nil, err
		}
		return e.updateChild(newChild, true)
	}
	eb := e.path[matched]
	enode, err := e.updatePath(e.path[matched+1:], true)
	if err != nil {
		return nil, err
	}
	lnode, err := newLeafNode(e.mpt, key, value)
	if err != nil {
		return nil, err
	}
	bnode, err := newBranchNode(
		e.mpt,
		map[byte]node{
			eb:                  enode,
			key[offset+matched]: lnode,
		},
	)
	if err != nil {
		return nil, err
	}
	if matched == 0 {
		return bnode, nil
	}
	return newExtensionNode(e.mpt, key[offset:offset+matched], bnode)
}

func (e *extensionNode) Search(key keyType, offset uint8) (node, error) {
	trieMtc.WithLabelValues("extensionNode", "search").Inc()
	matched := e.commonPrefixLength(key[offset:])
	if matched != uint8(len(e.path)) {
		return nil, trie.ErrNotExist
	}

	return e.child.Search(key, offset+matched)
}

func (e *extensionNode) proto() (proto.Message, error) {
	trieMtc.WithLabelValues("extensionNode", "serialize").Inc()
	h, err := e.child.Hash()
	if err != nil {
		return nil, err
	}
	return &triepb.NodePb{
		Node: &triepb.NodePb_Extend{
			Extend: &triepb.ExtendPb{
				Path:  e.path,
				Value: h,
			},
		},
	}, nil
}

func (e *extensionNode) Child() node {
	trieMtc.WithLabelValues("extensionNode", "child").Inc()
	return e.child
}

func (e *extensionNode) commonPrefixLength(key []byte) uint8 {
	return commonPrefixLength(e.path, key)
}

func (e *extensionNode) updatePath(path []byte, h bool) (node, error) {
	if err := e.delete(); err != nil {
		return nil, err
	}
	e.path = path

	hn, err := e.store()
	if err != nil {
		return nil, err
	}
	if h {
		return hn, nil
	}
	return e, nil
}

func (e *extensionNode) updateChild(newChild node, h bool) (node, error) {
	err := e.delete()
	if err != nil {
		return nil, err
	}
	e.child = newChild

	hn, err := e.store()
	if err != nil {
		return nil, err
	}
	if h {
		return hn, nil
	}
	return e, nil
}
