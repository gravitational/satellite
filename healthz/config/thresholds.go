package config

import (
	"sort"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
)

const defaultThreshold = 10

// Thresholds stores intervals of
type Thresholds struct {
	NodeCounts          []int
	UnavailablePercents map[int]int
}

// NewThresholds creates new Thresholds from string
func NewThresholds(unparsed string) (*Thresholds, error) {
	t := Thresholds{}
	t.UnavailablePercents = make(map[int]int)
	for _, unparsedThresh := range strings.Split(unparsed, ",") {
		a := strings.Split(unparsedThresh, ":")
		if len(a) != 2 {
			return nil, trace.Errorf("Unable to parse")
		}
		nodeCount, err := strconv.Atoi(a[0])
		if err != nil {
			return nil, trace.Wrap(err)
		}
		unavailablePercent, err := strconv.Atoi(a[1])
		if err != nil {
			return nil, trace.Wrap(err)
		}
		t.NodeCounts = append(t.NodeCounts, nodeCount)
		t.UnavailablePercents[nodeCount] = unavailablePercent
	}
	sort.Sort(sort.IntSlice(t.NodeCounts))
	return &t, nil
}

// GetByNodeCount finds threshold for specified point
func (t Thresholds) GetByNodeCount(nodeCount int) int {
	thresh := defaultThreshold
	for _, val := range t.NodeCounts {
		// Lot of assignments but simple logic
		if nodeCount >= val {
			thresh = t.UnavailablePercents[val]
		} else {
			break
		}
	}
	return thresh
}
