// 图的相关操作
package partition

// 图中的结点，即区块链网络中参与交易的账户
type Vertex struct {
	addr string // 账户地址
	// 其他属性待补充
}

// 描述当前区块链交易集合的图
type Graph struct {
	vertexSet map[Vertex]bool     // 节点集合，其实是 set
	edgeSet   map[Vertex][]Vertex // 记录节点与节点间是否存在交易，邻接表
	// lock      sync.RWMutex       //锁，但是每个储存节点各自存储一份图，不需要此
}

// 创建节点
func (v *Vertex) ConstructVertex(s string) {
	v.addr = s
}

// 增加图中的点
func (g *Graph) AddVertex(v Vertex) {
	if g.vertexSet == nil {
		g.vertexSet = make(map[Vertex]bool)
	}
	g.vertexSet[v] = true
}

// 增加图中的边
func (g *Graph) AddEdge(u, v Vertex) {
	// 如果没有点，则增加边，权恒定为 1
	if _, ok := g.vertexSet[u]; !ok {
		g.AddVertex(u)
	}
	if _, ok := g.vertexSet[v]; !ok {
		g.AddVertex(v)
	}
	if g.edgeSet == nil {
		g.edgeSet = make(map[Vertex][]Vertex)
	}
	// 无向图，使用双向边
	g.edgeSet[u] = append(g.edgeSet[u], v)
	g.edgeSet[v] = append(g.edgeSet[v], u)
}

// 复制图
func (dst *Graph) CopyGraph(src Graph) {
	dst.vertexSet = make(map[Vertex]bool)
	for v := range src.vertexSet {
		dst.vertexSet[v] = true
	}
	if src.edgeSet != nil {
		dst.edgeSet = make(map[Vertex][]Vertex)
		for v := range src.vertexSet {
			dst.edgeSet[v] = make([]Vertex, len(src.edgeSet[v]))
			copy(dst.edgeSet[v], src.edgeSet[v])
		}
	}
}

// 输出图
func (g Graph) PrintGraph() {
	for v := range g.vertexSet {
		print(v.addr, " ")
		print("edge:")
		for _, u := range g.edgeSet[v] {
			print(" ", u.addr, "\t")
		}
		println()
	}
	println()
}
