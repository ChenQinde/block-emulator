package test

import (
	"blockEmulator/partition"
)

func Test_graph() {
	var a, b, c, d, e, f partition.Vertex
	a.ConstructVertex("0x00000000")
	b.ConstructVertex("0x00000001")
	c.ConstructVertex("0x00000002")
	d.ConstructVertex("0x00000003")
	e.ConstructVertex("0x00000004")
	f.ConstructVertex("0x00000005")

	var h partition.CLPAState
	// g.AddVertex(a)
	// g.AddVertex(b)
	// g.AddVertex(c)
	// g.AddVertex(d)
	// g.AddVertex(e)

	h.NetGraph.AddEdge(a, b)
	h.NetGraph.AddEdge(a, b)
	h.NetGraph.AddEdge(a, b)
	h.NetGraph.AddEdge(a, b)
	h.NetGraph.AddEdge(a, b)
	h.NetGraph.AddEdge(a, b)
	h.NetGraph.AddEdge(b, c)
	h.NetGraph.AddEdge(a, c)
	h.NetGraph.AddEdge(a, c)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	h.NetGraph.AddEdge(c, d)
	err := h.Stable_Init_Partition()
	if err == nil {
		h.PrintCLPA()
		h.CLPA_Partition()
		h.PrintCLPA()
	}
}
