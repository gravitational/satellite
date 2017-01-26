package config

import (
	"strconv"
	"sort"
	"strings"

	"github.com/gravitational/trace"
)

const DefaultThreshold = 10

type Thresholds struct {
	NodeCounts []uint
	DeadPcts   map[uint]uint8
}

type uintSlice []uint

func (t uintSlice) Len() int {
	return len(t)
}

func (t uintSlice) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (t uintSlice) Less(i, j int) bool {
	return t[i] < t[j]
}

func NewThresholds(unparsed string) (*Thresholds, error) {
	t := Thresholds{}
	t.DeadPcts = make(map[uint]uint8)
	for _, unparsedThresh := range strings.Split(unparsed, ",") {
		a := strings.Split(unparsedThresh, ":")
		if len(a) != 2 {
			return nil, trace.Errorf("Unable to parse")
		}
		nodeCount, err := strconv.Atoi(a[0])
		if err != nil {
			return nil, trace.Wrap(err)
		}
		deadPct, err := strconv.Atoi(a[1])
		if err != nil {
			return nil, trace.Wrap(err)
		}
		t.NodeCounts = append(t.NodeCounts, uint(nodeCount))
		t.DeadPcts[uint(nodeCount)] = uint8(deadPct)
	}
	sort.Sort(uintSlice(t.NodeCounts))
	return &t, nil
}

func (t Thresholds) GetByNodeCount(nodeCount uint) uint8 {
	var thresh uint8 = DefaultThreshold
	for _, val := range t.NodeCounts {
		// Lot of assignments but simple logic
		if nodeCount >= val {
			thresh = t.DeadPcts[val]
		} else {
			break
		}
	}
	return thresh
}
