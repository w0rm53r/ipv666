package modeling

import (
	"fmt"
	"github.com/lavalamp-/ipv666/internal/addressing"
	"github.com/lavalamp-/ipv666/internal/logging"
	"github.com/lavalamp-/ipv666/internal/persist"
	"net"
)

type AddressTree struct {
	ChildrenCount		int64						`json:"count"`
	Children			map[uint8]*AddressTreeNode	`json:"children"`
}

type AddressTreeNode struct {
	ChildrenCount		int64						`json:"count"`
	Children			map[uint8]*AddressTreeNode	`json:"children"`
	Depth				uint8						`json:"depth"`
}

func newAddressTree() *AddressTree {
	return &AddressTree{
		ChildrenCount:	0,
		Children:		make(map[uint8]*AddressTreeNode),
	}
}

func newAddressTreeNode(depth uint8) *AddressTreeNode {
	return &AddressTreeNode{
		ChildrenCount:	0,
		Children:		make(map[uint8]*AddressTreeNode),
		Depth:			depth,
	}
}

func CreateFromAddresses(toAdd []*net.IP, emitFreq int) *AddressTree {
	toReturn := newAddressTree()
	toReturn.AddIPs(toAdd, emitFreq)
	return toReturn
}

func (addrTree *AddressTree) AddIP(toAdd *net.IP) bool {
	ipNybbles := addressing.GetNybblesFromIP(toAdd, 32)
	if addrTree.containsIPByNybbles(ipNybbles) {
		return false
	}
	if _, ok := addrTree.Children[ipNybbles[0]]; !ok {
		addrTree.Children[ipNybbles[0]] = newAddressTreeNode(0)
	}
	addrTree.Children[ipNybbles[0]].addNybbles(ipNybbles[1:])
	addrTree.ChildrenCount++
	return true
}

func (addrTree *AddressTree) AddIPs(toAdd []*net.IP, emitFreq int) (int, int) {
	added, skipped := 0, 0
	for i, curAdd := range toAdd {
		if i % emitFreq == 0 {
			logging.Infof("Adding IP address %d out of %d to address tree.", i, len(toAdd))
		}
		if addrTree.AddIP(curAdd) {
			added++
		} else {
			skipped++
		}
	}
	return added, skipped
}

func (addrTree *AddressTree) GetAllIPs() []*net.IP {
	if addrTree.ChildrenCount == 0 {
		return []*net.IP{}
	} else {
		var toReturn []*net.IP
		for k, v := range addrTree.Children {
			toReturn = append(toReturn, v.getAllIPs([]uint8{k})...)
		}
		return toReturn
	}
}

func (addrTree *AddressTree) seekChildByNybbles(nybbles []uint8) (*AddressTreeNode, error) {
	if val, ok := addrTree.Children[nybbles[0]]; !ok {
		return nil, nil
	} else {
		return val.seekNode(nybbles[1:]), nil
	}
}

func (addrTree *AddressTree) getSeekNybbles(fromRange *net.IPNet) ([]uint8, error) {
	ones, _ := fromRange.Mask.Size()
	if ones % 4 != 0 {
		return nil, fmt.Errorf("cannot get IPs from a network range that isn't on a nybble boundary (ie: modulo 4, mask size was %d)", ones)
	} else {
		return addressing.GetNybblesFromIP(&fromRange.IP, ones / 4), nil
	}
}

func (addrTree *AddressTree) GetIPsInRange(fromRange *net.IPNet) ([]*net.IP, error) {
	networkNybbles, err := addrTree.getSeekNybbles(fromRange)
	if err != nil {
		return nil, err
	}
	if len(networkNybbles) == 0 {
		return addrTree.GetAllIPs(), nil
	}
	child, err := addrTree.seekChildByNybbles(networkNybbles)
	if err != nil {
		return nil, err
	} else {
		return child.getAllIPs(networkNybbles), nil
	}
}

func (addrTree *AddressTree) CountIPsInRange(fromRange *net.IPNet) (int64, error) {
	networkNybbles, err := addrTree.getSeekNybbles(fromRange)
	if err != nil {
		return -1, err
	}
	if len(networkNybbles) == 0 {
		return addrTree.ChildrenCount, nil
	} else if len(networkNybbles) == 32 {
		if addrTree.containsIPByNybbles(networkNybbles) {
			return 1, nil
		} else {
			return 0, nil
		}
	}
	child, err := addrTree.seekChildByNybbles(networkNybbles)
	if err != nil {
		return -1, err
	} else {
		return child.ChildrenCount, nil
	}
}

func (addrTree *AddressTree) Save(filePath string) error {
	return persist.Save(filePath, addrTree)
}

func LoadAddressTreeFromFile(filePath string) (*AddressTree, error) { // TODO abstract this away
	var toReturn AddressTree
	err := persist.Load(filePath, &toReturn)
	return &toReturn, err
}

func (addrTree *AddressTree) ContainsIP(toCheck *net.IP) bool {
	nybs := addressing.GetNybblesFromIP(toCheck, 32)
	return addrTree.containsIPByNybbles(nybs)
}

func (addrTree *AddressTree) containsIPByNybbles(nybbles []uint8) bool {
	if val, ok := addrTree.Children[nybbles[0]]; !ok {
		return false
	} else {
		return val.containsNybbles(nybbles[1:])
	}
}

func (addrTreeNode *AddressTreeNode) addNybbles(nybbles []uint8) {
	if len(nybbles) == 0 {
		return
	} else if _, ok := addrTreeNode.Children[nybbles[0]]; !ok {
		addrTreeNode.Children[nybbles[0]] = newAddressTreeNode(addrTreeNode.Depth + 1)
	}
	addrTreeNode.Children[nybbles[0]].addNybbles(nybbles[1:])
	addrTreeNode.ChildrenCount++
}

func (addrTreeNode *AddressTreeNode) containsNybbles(nybbles []uint8) bool {
	if val, ok := addrTreeNode.Children[nybbles[0]]; !ok {
		return false
	} else if len(nybbles) == 1 {
		return true
	} else {
		return val.containsNybbles(nybbles[1:])
	}
}

func (addrTreeNode *AddressTreeNode) getAllIPs(parentNybbles []uint8) []*net.IP {
	if len(addrTreeNode.Children) == 0 && addrTreeNode.Depth != 31 {
		logging.Warnf("Ran out of children at depth %d when getting all IPs. This shouldn't happen.", addrTreeNode.Depth)
		return []*net.IP{}
	} else if len(addrTreeNode.Children) == 0 {
		toAdd := addressing.NybblesToIP(parentNybbles)
		return []*net.IP{ toAdd }
	} else {
		var toReturn []*net.IP
		for k, v := range addrTreeNode.Children {
			toReturn = append(toReturn, v.getAllIPs(append(parentNybbles, k))...)
		}
		return toReturn
	}
}

func (addrTreeNode *AddressTreeNode) getAllIPsInRange(parentNybbles []uint8, searchNybbles []uint8) []*net.IP {
	if len(searchNybbles) == 0 {
		return addrTreeNode.getAllIPs(parentNybbles)
	} else if val, ok := addrTreeNode.Children[searchNybbles[0]]; !ok {
		return []*net.IP{}
	} else {
		return val.getAllIPsInRange(append(parentNybbles, searchNybbles[0]), searchNybbles[1:])
	}
}

func (addrTreeNode *AddressTreeNode) seekNode(seekNybbles []uint8) *AddressTreeNode {
	if len(seekNybbles) == 0 {
		return addrTreeNode
	} else if val, ok := addrTreeNode.Children[seekNybbles[0]]; !ok {
		return nil
	} else {
		return val.seekNode(seekNybbles[1:])
	}
}
