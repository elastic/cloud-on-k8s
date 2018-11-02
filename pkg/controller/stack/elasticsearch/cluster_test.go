package elasticsearch

import "testing"

func TestComputeMinimumMasterNodes(t *testing.T) {
	test := func(nodeCount, expected int) {
		actual := ComputeMinimumMasterNodes(nodeCount)
		if actual != expected {
			t.Errorf("With nodeCount=%d: expected %d, actual %d", nodeCount, expected, actual)
		}
	}
	test(1, 1)
	test(2, 2)
	test(3, 2)
	test(4, 3)
	test(5, 3)
	test(6, 4)
	test(100, 51)
}
