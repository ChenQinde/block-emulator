# BrokerChain在BlockEmulator实现

> 🎯 我们时常遇到一些令人叹为观止的机遇，却一次次地被当成不可能解决的问题    -- 約翰．加德納

# 一、项目介绍

> blockEmulator的项目介绍👉[blockEmulator项目开发](https://j4s9dl19cd.feishu.cn/docs/doccnvzanUrYNyNZjjnvCuGR2Uc#) 

### 在BlockEmulator上实现BrokerChain

![img](https://j4s9dl19cd.feishu.cn/space/api/box/stream/download/asynccode/?code=MDdmOWE5ZTZjMzdmZjdkNTI0ZmM0YjY1YjI4ZTIyMDFfdTlQUUswR1NJV21FajRKTEtoQ09MVHJsSlJib204em1fVG9rZW46Ym94Y25vNGZOQUR3VEdvYjZ5WklHNXdOY3JlXzE2NjYxMDQ3ODU6MTY2NjEwODM4NV9WNA)

图 1. BlockEmulator目标

**broker-to-earn：**设计一个 激励机制，吸引某些用户资源成为 broker Accounts，通过为系统做贡献，而获取一定的佣金收入。

# 二、项目目标

**目标一：分片模块的修改、功能新增。**

**目标二：账户模块账户模块的实现。**

**目标三：时间模块的实现。**

**目标四：BlockEmulator进阶功能的实现。**

# 二、什么是BrokerChain？

**BrokerChain简要介绍：****论文链接**

![img](https://j4s9dl19cd.feishu.cn/space/api/box/stream/download/asynccode/?code=MzU5MzBiMzFhOWFkMzM2ODA3ZGM5ZjliNWIwNmUyMmNfQWdVSjNHaElEbDhqcXIyUXR0ZWpEV0w4a2JibFFIb2VfVG9rZW46Ym94Y25mT0E4blRQVDlPU2NCWVdqRzBycmhlXzE2NjYxMDQ3ODU6MTY2NjEwODM4NV9WNA)

图 2. **BrokerChain四个阶段**

**BrokerChain提出了：**

1. 一种**新的跨分片交易处理协议**，引入“做市商账户”（Broker账户）来减少跨分片交易的数量，以及采用“双Nonce”机制以及“部分时间状态锁”来防止双花交易。

1. 一种**新的状态划分方案**，分片划分方案根据一定时间内的历史交易信息构建一个账户交易状态图，并对其进行图划分，以此来平衡各个分片的交易数量以及减少跨分片交易的数量。

 

**BrokerChain四个主要阶段如上图2：**

1. M分片打包来自交易池的交易，并且通过PBFT生成交易块。

1. P分片读取M分片生成块的交易，持续更新所有账户的状态图。

1. 根据P分片生成的状态图进行PBFT共识以及账户划分（Account Segmentation）。

1. 根据P分片达成共识(PBFT)的状态图对于下一个epoch的M分片进行重新划分。

# 三、项目具体内容

**BrokerChain在blockEmulator上使用，需要实现/修改的部分（总）：**

1. **分片模块：**
   1. <font color =red>自由生成自定义分片</font>>（根据给定参数能够生成<font color =red>指定数量</font>、<font color =red>指定划分</font>等功能）
   2. <font color =red>根据历史交易生成状态图</font>（状态图的格式、状态图边点的权重、读取历史交易等）
   3. <font color =red>根据给定状态图调整分片的功能</font>（除最初epoch以外，每一个epoch开始都会根据状态图来调整分片）
   4. <font color =red>P分片及其链的实现</font>

1. **账户模块（<font color =red>目前未实现，需新增</font>**）:
   1. <font color =red>引用以太坊account包</font>，包括钱包功能等。
   2. <font color =red>对于Broker账户的挑选</font>
      - 目前的方案：挑选测试数据中交易数量最多的账户作为Broker账户。

1. **时间模块（<font color =red>目前未实现，需新增</font>**）：
   1. 目前代码暂时没有对于链模拟的时间记录（比如BrokerChain的epoch），暂时局限于一个epoch内（即比如pbft中没有切换视图等）。
   2. 新增时间轴，把目前单epoch的模拟拓展到多epoch的模拟。

1. **进阶功能（<font color =red>目前未实现，需新增</font>**）：
   1. 替换分片内共识。
   2. 实现中心化分片（烨彤在做这部分）。

# 四、以BrokerChain角度描述需实现部分

**BrokerChain需要实现的部分，按照上诉四个阶段进行划分细致一些的描述：**

1. **M分片打包来自交易池的交易，并且通过PBFT生成交易块。**
   1. M分片视为正常分片即可，及对于日常交易进行处理打分的分片
   2. 以BlockEmulator现有的对于分片的支持，预估需要对shard模块调整下面功能：
      - <font color =red>自由生成自定义分片</font>（根据给定参数能够生成<font color =red>指定数量</font>、<font color =red>指定划分</font>等功能）

1. **P分片读取M分片生成块的交易，持续更新所有账户的状态图。**
   1. P分片为特殊的分片，<font color =red>根据历史交易生成账户状态图</font>来对下一个epoch的M分片进行调整。
   2. 以BlockEmulator现有的对于分片的支持，预估需要对shard模块调整下面功能：
      - <font color =red>根据历史交易生成状态图</font>（状态图的格式、状态图边点的权重、读取历史交易等）

1. **根据P分片生成的状态图进行PBFT共识以及账户划分（Account Segmentation）。**
   1. 对于P生成的状态图进行PBFT。
   2. <font color =red>对于Broker账户的挑选</font>
      - 目前的方案：挑选测试数据中交易数量最多的账户作为Broker账户。
   3. 以BlockEmulator已有功能，需要新增**账户模块（目前BlockEmulator没有实现账户功能）：**
      - 引用以太坊account包，包括钱包功能等。

1. **根据P分片达成共识的状态图对于下一个epoch的M分片进行重新划分。**
   1. 对于<font color =red>已达成共识的状态图上P分片的链</font>。
   2. 以BlockEmulator现有的对于分片的支持，预估需要对shard模块调整下面功能： 
      - 1. <font color =red>根据给定状态图调整分片的功能</font>（除最初epoch以外，每一个epoch开始都会根据状态图来调整分片）

1. **其他部分**
   1. 时间模块，brokerChain以epoch为单位（目前以NTXs个交易为单位进行切换）

# 五、任务拆解

> <font color =red>注意</font>>：brokerChain独有的部分在brokerChain文件夹下实现

| 任务                   | 任务状态 | 开始时间   | 截止时间   |
| ---------------------- | -------- | ---------- | ---------- |
| 自由生成自定义分片功能 | ❇️ 已完成 | 2022-10-12 | 2022-10-19 |
| 进行部分重构           | 🔛进行中  | 2022-10-20 | --         |
| 进行中账户模块的实现   | 🔵待启动  |            |            |
| 讨论状态图的具体细节   | 🔵待启动  |            |            |
| 状态图模块实现         | 🔵待启动  |            |            |
| M分片根据状态图更新    | 🔵待启动  |            |            |
| P链的实现              | 🔵待启动  |            |            |
| 日志模块               | 🔵待启动  |            |            |
| 时间模块               | 🔵待启动  |            |            |
| 拓展功能               | 🔵待启动  |            |            |

# 六、Usage

## 1. 自由分片功能

### （1）启动客户端（仿真者）

打开一个新的终端，进入项目文件夹，运行以下命令： 

```PowerShell
go run main.go utils.go -c -t [path]
```

这意味着启动客户端。

### （2）启动分片 

打开一个新的终端，进入项目文件夹，运行以下命令： （**ps : 下面的测试文件path需要和客户端的一致**）

``` 
go run main.go utils.go -S 1 -N 4 -t [path]
```

这意味着启动1个分片，每个分片具备4个节点，其中有一个节点为主节点，默认为0号节点。

# 七、更新日志

2022-10-18： 更新了自由分片功能，但似乎还有bug

2022-10-19： 修复了自由分片功能的bug

bug源于

- 1. 修改后的分片在开始运行之后才生出分片的表格，并且存储于params/config.go中（运行内存中）
- 2. 分片结束运行源自于客户端发送的分片结束信号，而不是所有交易结束之后自动结束。（分片仍然在打包没有交易的空区块）

修改记录：

- 1. 将分片分好之后，存储于params/nodeTable.json下，即客户端于不同终端启动都能够读取存在json文件中的节点表格。

仍然存在问题：

- 1. 客户端定位仍然尴尬，客户端的作用应该修改为可以观察、输入交易等，比如我执行一段交易，然后进行分片中运行看看需要多少时间执行。（ps：观察交易这一块功能可以用于后面P链的实现。）
- 2. ~~分片的结束运行应该由分片自己决定，而不是由客户端发送终止信息。~~（20221022）
- 3. ~~分片在没有交易的时候，是否不应该继续打包空区块？~~（20221022）

2022-10-22： 

1. 修复了自由分片功能多分片通讯bug。

2. 增加了文件夹不存在则创建功能，log文件夹自动创建。
3. 区块数据存储位置由根目录调整至新文件夹record中。
4. 节点终止信号由原本客户端发送改为在主节点发起propose时，如果出现交易数量为0的情况，则向本分片所有节点（包括主节点自己）发送终止信号。
5. 进行了一部分重构。
6. 暂时性的移除客户端在此处起的作用。

2022-12-20： 

1. 把客户端（仿真者）重新独立出来
2. 修改了自由分片功能的一些bug
   - 1. 不同分片的日志读写问题
     2. monoxide跨分片策略在自由分配情况下会出现的问题
3. pbft作为默认M分片的（很多东西还没有修改，比如状态树等，这些得根据到时候实现的账户模块等来进行具体的修改）

2022-12-21:

1. 实现了 Graph ，用来描绘网络交易拓扑
2. 实现了 CLPA partition 算法，可以用它进行账户划分
   - 将交易作为图（Graph）的边插入图中
   - 运行初始划分算法 Stable_Init_Partition（保证不会出现空分片）
   - 运行 CLPA 算法
   - 账户划分的结果保存在 CLPAstate 的 PartitionMap 下
3. test_graph 为一个样例