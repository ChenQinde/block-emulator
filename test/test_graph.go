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
	h.Init_CLPAState(0.5, 100, 2)
	// g.AddVertex(a)
	// g.AddVertex(b)
	// g.AddVertex(c)
	// g.AddVertex(d)
	// g.AddVertex(e)

	h.AddEdge(a, b)
	h.AddEdge(a, b)
	h.AddEdge(a, b)
	h.AddEdge(a, b)
	h.AddEdge(a, b)
	h.AddEdge(a, b)
	h.AddEdge(b, c)
	h.AddEdge(a, c)
	h.AddEdge(a, c)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	h.AddEdge(c, d)
	err := h.Stable_Init_Partition()
	if err == nil {
		h.PrintCLPA()
		h.CLPA_Partition()
		h.PrintCLPA()
	}
}
