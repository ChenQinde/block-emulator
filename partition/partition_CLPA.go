package partition

import (
	"log"
	"strconv"
)

// CLPA算法状态，state of constraint label propagation algorithm
type CLPAState struct {
	NetGraph          Graph          // 需运行CLPA算法的图
	PartitionMap      map[Vertex]int // 记录分片信息的 map
	Edges2Shard       []int          // Shard 相邻接的边数，对应论文中的 total weight of edges associated with label k
	VertexsNumInShard []int          // Shard 内节点的数目
	weightPenalty     float64        // 权重惩罚，对应论文中的 beta
	minEdges2Shard    int            // 最少的 Shard 邻接边数，最小的 total weight of edges associated with label k
	maxIterations     int            // 最大迭代次数，constraint，对应论文中的\tau
	shardNum          int            // 分片数目
}

// 复制CLPA状态
func (dst *CLPAState) CopyCLPA(src CLPAState) {
	dst.NetGraph.CopyGraph(src.NetGraph)
	dst.PartitionMap = make(map[Vertex]int)
	for v := range src.PartitionMap {
		dst.PartitionMap[v] = src.PartitionMap[v]
	}
	dst.Edges2Shard = make([]int, src.shardNum)
	copy(dst.Edges2Shard, src.Edges2Shard)
	dst.VertexsNumInShard = src.VertexsNumInShard
	dst.weightPenalty = src.weightPenalty
	dst.minEdges2Shard = src.minEdges2Shard
	dst.maxIterations = src.maxIterations
	dst.shardNum = src.shardNum
}

// 输出CLPA
func (cs *CLPAState) PrintCLPA() {
	cs.NetGraph.PrintGraph()
	println(cs.minEdges2Shard)
	for v, item := range cs.PartitionMap {
		print(v.addr, " ", item, "\t")
	}
	for _, item := range cs.Edges2Shard {
		print(item, " ")
	}
	println()
}

// 根据当前划分，计算 Wk，即 Edges2Shard
func (cs *CLPAState) computeEdges2Shard() {
	cs.Edges2Shard = make([]int, cs.shardNum)
	cs.minEdges2Shard = 0x7fffffff // INT_MAX

	for v, lst := range cs.NetGraph.edgeSet {
		// 获取节点 v 所属的shard
		vShard := cs.PartitionMap[v]
		for _, u := range lst {
			// 同上，获取节点 u 所属的shard
			uShard := cs.PartitionMap[u]
			if vShard != uShard {
				// 判断节点 v, u 不属于同一分片，则对应的 Edges2Shard 加一
				// 仅计算入度，这样不会重复计算
				cs.Edges2Shard[uShard] += 1
			}
		}
	}
	// 修改 minEdges2Shard
	for _, val := range cs.Edges2Shard {
		if cs.minEdges2Shard > val {
			cs.minEdges2Shard = val
		}
	}
}

// 设置参数
func (cs *CLPAState) Set_Parameters(wp float64, mIter, sn int) {
	cs.weightPenalty = wp
	cs.maxIterations = mIter
	cs.shardNum = sn
}

// 初始化划分，使用节点地址的尾数划分，应该保证初始化的时候不会出现空分片
func (cs *CLPAState) Init_Partition() {
	// 设置划分默认参数
	cs.Set_Parameters(0.5, 100, 2)
	cs.VertexsNumInShard = make([]int, cs.shardNum)
	cs.PartitionMap = make(map[Vertex]int)
	for v := range cs.NetGraph.vertexSet {
		var va = v.addr[len(v.addr)-5:]
		num, err := strconv.ParseInt(va, 16, 32)
		if err != nil {
			log.Panic()
		}
		cs.PartitionMap[v] = int(num) % cs.shardNum
		cs.VertexsNumInShard[cs.PartitionMap[v]] += 1
	}
	// 账户划分完成之后，计算 Edges2Shard
	cs.computeEdges2Shard()
}

// 计算 将节点 v 放入 uShard 所产生的 score
func (cs *CLPAState) getShard_score(v Vertex, uShard int) float64 {
	var score float64
	// 节点 v 的出度
	v_outdegree := len(cs.NetGraph.edgeSet[v])
	// uShard 与节点 v 相连的边数
	Edgesto_uShard := 0
	for _, item := range cs.NetGraph.edgeSet[v] {
		if cs.PartitionMap[item] == uShard {
			Edgesto_uShard += 1
		}
	}
	score = float64(Edgesto_uShard) / float64(v_outdegree) * (1 - cs.weightPenalty*float64(cs.Edges2Shard[uShard])/float64(cs.minEdges2Shard))
	return score
}

// CLPA 划分算法
func (cs *CLPAState) CLPA_Partition() {
	for iter := 1; iter < cs.maxIterations; iter += 1 { // 第一层循环控制算法次数，constraint
		stop := true // stop 控制算法是否提前停止
		for v := range cs.NetGraph.vertexSet {
			neighborShardScore := make(map[int]float64)
			max_score := -9999.0
			vNowShard, max_scoreShard := cs.PartitionMap[v], cs.PartitionMap[v]
			for _, u := range cs.NetGraph.edgeSet[v] {
				uShard := cs.PartitionMap[u]
				// 对于属于 uShard 的邻居，仅需计算一次
				if _, computed := neighborShardScore[uShard]; !computed {
					neighborShardScore[uShard] = cs.getShard_score(v, uShard)
					if max_score < neighborShardScore[uShard] {
						max_score = neighborShardScore[uShard]
						max_scoreShard = uShard
					}
				}
			}
			if vNowShard != max_scoreShard && cs.VertexsNumInShard[vNowShard] > 1 {
				cs.PartitionMap[v] = max_scoreShard
				// 重新计算 VertexsNumInShard
				cs.VertexsNumInShard[vNowShard] -= 1
				cs.VertexsNumInShard[max_scoreShard] += 1
				// 重新计算Wk
				cs.computeEdges2Shard()
				stop = false
				break
			}
		}
		// 当本次循环无节点进行迁移时，break
		if stop {
			break
		}
	}
}
