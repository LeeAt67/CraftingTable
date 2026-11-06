# 应用内知识库调研：从 Documents 演进为可检索知识库

日期：2026-11-06
状态：调研结论（未进入设计）
范围：CraftingTable 应用内知识库的能力边界、触发条件、数据模型方向；以 MiCO wiki 的真实实现为对照基线
排除：Agent 自动写入知识库、多人协同编辑、跨租户权限
关联需求：[K-01](../requirements.md)、[K-02](../requirements.md)
依据：MiCO 仓库 `~/repos/mico`（`main` @ `fb894a8f639`）源码与设计文档、本仓库 `feat/docs-writing` 分支产物、[测试策略调研](./2026-11-06-testing-strategy.md)

> 本文回答三个问题：MiCO 的知识库真实是怎么实现的、CraftingTable 该抄什么不该抄什么、什么条件下才值得动手。

## 1. 结论

**现在不该做知识库，但该现在就定"什么条件下做"。**

理由不是"太复杂"，而是：CraftingTable 的 Documents 工作区还没进主分支（`feat/docs-writing` 仍在独立分支），连"文档能稳定保存和检索"都还没有基线。在基线之上再叠一层"知识库"，等于在一栋没验收的楼上加层。

同时，MiCO 的实现给出了一个**极有价值的反例**：它的知识库不是"文档 + 全文搜索"，而是"文档 → 蒸馏页 → 知识图谱"三层，配套 22 个 CLI 子命令、外部数据源同步、人工纠错工作流。这个体量对一个多租户企业产品成立，对单用户本地应用则是明显过度设计。

**真正该立刻借鉴的是三条与体量无关的设计**（§6）：作用域锚定、原文/蒸馏双通道、渐进式披露。

## 2. MiCO 知识库的真实架构

先说清分层，否则容易把 UI、BFF 和真正的知识服务混为一谈。

```text
mico-wiki（Module Federation 子应用，React 19 + rsbuild）
  挂在 mico 宿主的 /wiki 路由，remote name = mico_wiki
        ↓ HTTP
mico（宿主，Next.js + Go）：BFF、鉴权、工作区设置、成员同步
        ↓ X5 签名内部协议
mi-knowledge-service（Java，8085）：文档 / 目录 / 检索 / 图谱
mi-knowledge-collab（Java，8086）：实时协同编辑
```

依据：`docs/wiki-subapp-dev-guide.md` 明确写了「知识库的 wiki 页面代码不在本仓（mico），而在独立仓库 `mico-wiki`」，并列出四个仓库的分工与端口。`server/internal/mico/knowledgeplatform/` 里有 `java_authorization_live_test.go`、`java_compatibility_live_test.go`，印证对端是 Java 服务。

这个分层本身就是一条经验：**知识库的业务逻辑应该是一个独立服务，而不是塞进宿主后端的某个 package**。CraftingTable 是模块化单体，对应做法是独立模块 + 明确 port，而不是散在 `today` 或 `agent` 里。

### 2.1 三层内容模型

| 层 | 载体 | 证据 |
| --- | --- | --- |
| 原始文档 | `document` + `document_chunk` | `docs/superpowers/specs/2026-09-16-mico-wiki-search-raw-text-flag-design.md`：`text → document_chunk` |
| 蒸馏页 | `page`，类型 `entity` / `topic` / `document` / `directory` | `mico wiki search --page-type`；`mico wiki page-get`（"distill page"） |
| 图谱 | `edge`，可编辑可删除 | `mico wiki edge-list`、`correction-submit --mode edge_edit\|edge_delete` |

关键事实：**文档不是检索的最小单位，chunk 才是**。`SearchResult` 结构里有 `DocumentID`、`ChunkID`、`ChunkIndex`、`Snippet`、`Body`，注释写明「Body is the full chunk content; Snippet is its truncated preview (first ~240 chars upstream). Agents get uncut context via Body」。

这解释了一个常见困惑：为什么搜索结果里既有文档又有 chunk id——因为同一个文档的不同片段可能分别命中。

### 2.2 两种库模式与按请求覆盖

```go
// server/internal/mico/knowledge/models.go
type CreateLibraryRequest struct {
    Mode string `json:"mode"` // "text" | "hybrid"
    ...
}
```

- `mode` 在**建库时固定**，请求不能改。
- 早期问题：hybrid 模式下蒸馏结果召回量大，会淹没原文 chunk；Agent 需要精确细节（数字、表格、cell 级数据）时拿不到。
- 解法（2026-09-16 决策）：新增 per-request `--source text`，当次请求强制走 `document_chunk`，跳过 atlas 蒸馏。

```go
// SearchRequest.Source 的注释
// Source is a per-request override: "text" forces document_chunk search
// (skips atlas distill) even when library.mode=hybrid. Empty = respect
// library.mode (default behavior). NOT a reuse of Mode
```

**这条设计值得直接吸收**：摘要层和原文层服务于不同问题，必须都能按需访问，而不是二选一。

### 2.3 检索契约的细节

`SearchRequest` 的字段揭示了完整能力面：

| 字段 | 作用 |
| --- | --- |
| `QueryPage{ID,Type,Title}` | 把召回**偏向**某一页（entity / topic）。注释：「prioritizes chunks belonging to the specified page」 |
| `Filters{DirectoryIDs, FullPathPrefix, SourceTypes}` | 范围过滤：目录、路径前缀、来源类型 |
| `Page` / `PageSize` | 分页 |
| `InvokeSource` | `agent_auto` / `user_hash_tag` / `user_plus_menu`，用于区分调用来源 |

`InvokeSource` 这个字段很说明问题：**同一个检索接口，Agent 触发和用户主动触发被显式区分**。这为后续按来源调参、做效果归因留好了口子。

### 2.4 外部数据源同步

`mico wiki ds-*` 支持 `git`、`feishu_wiki`、`feishu_doc`、`gitlab`、`meeting`、`local_file`、`custom`。

| 设计点 | 做法 | 为什么值得注意 |
| --- | --- | --- |
| 默认自动同步 | `--auto-sync` 默认 true，加入每日 00:00 | 知识库的价值在"不需要手动维护" |
| 删除不级联 | `ds-delete`「does not cascade to documents」 | 源没了但内容仍在，避免误删已整理的知识 |
| 大导入保护 | `--confirm-scope` | 显式确认范围 |
| 手动触发需权限 | `ds-sync` 需 library admin 角色 | 写操作与读操作分级 |

### 2.5 人工纠错

```bash
mico wiki correction-submit --mode entity_full_edit | entity_span_edit
                                 | entity_merge | edge_edit | edge_delete
                           --instruction "用户原始诉求"   # 必填
```

- 五种模式覆盖：整页重写、片段改写、实体合并、改边、删边。
- `--instruction` 必填 = **每次纠错都记录动机**，这是把人工反馈变成可训练信号的唯一低成本方式。
- 「commits on submit」——提交即生效，无审批队列。

### 2.6 代码也是一种知识资产（Atlas）

`mico wiki code` 把代码结构与文档放进同一套检索：

| 命令 | 能力 |
| --- | --- |
| `overview` | 模块 / 依赖 / 入口 / 热点 / 聚类 |
| `explore` | 自然语言找符号，**RRF 多路融合**，`--semantic` 才加向量 |
| `trace` | 调用图 BFS，可指定深度与方向 |
| `impact` | 改动影响面（inbound callers + downstream） |
| `snippet` | 按限定名取源码 |
| `tour` | 针对查询给出"值得读的节点"导览 |
| `adr-general` | `delivery_policy=always_on` 的规则，常驻下发 |

`adr-general` 本质上就是"存在服务端的 AGENTS.md"。这条对 CraftingTable 有直接启发：**规则可以不止放在仓库文件里，也可以作为一种带版本的检索资产**。

## 3. 三条最有价值的经验（与体量无关）

### 3.1 作用域锚定：Agent 必须按最窄上下文行动

MiCO 的 `docs/adr/0012-knowledge-context-scope-anchoring.md` 记录了一次真实事故：

> 用户在一篇文档里选中一句话，让 Agent「删除这句」，Agent 却删掉了**整篇文档**。

根因是三个缺陷叠加：

1. `doc-delete <doc-id>` 是无防护的整篇删除，而 doc-id 在页面上下文里永远存在。
2. 文档层没有"删/改一句"的 span 工具，只有整篇覆盖和整篇删除。
3. skill 的模式判定表把动词「删除」硬绑到 `edge_delete`，"删除 + 有 selection"这个组合没有任何规则接住。

最终决策是五条铁律：

| 上下文 | 允许的作用域 |
| --- | --- |
| 有 `selection` | 只作用于选中片段，**绝不升级**为整篇或整实体的删除/重写 |
| 无 selection、有 `page-id` | 只作用于该实体 |
| 无 selection、有 `doc-id` | 只作用于该文档 |
| 无 selection、有 `node-id` | 只作用于该节点对应的文档或目录 |
| 破坏性整体操作 | 必须①用户明确点名整篇/整实体 ②二次确认 |

**这一条对 CraftingTable 是硬需求，不是可选优化。** 我们计划让 Agent 读今日条目和文档（AG-01 已完成只读工具），一旦开放写操作（AG-03），同类事故必然发生。差别只在于 MiCO 有 22 个命令可误用，我们有更少的命令但同样会误用。

### 3.2 原文与摘要是两条通道

MiCO 用了半年才发现"hybrid 模式下蒸馏淹没原文"是个真问题。这个教训可以直接前移：**如果要做摘要层，第一天就保留原文通道的开关**。

### 3.3 Skill 的渐进式披露

`docs/superpowers/specs/2026-09-16-mico-wiki-skill-progressive-disclosure-design.md` 把 skill 拆成三层：

| 层 | 内容 | Token 成本 | 何时加载 |
| --- | --- | --- | --- |
| 元数据 | `name` + `description` | ~100 | 启动时全部加载，用于路由 |
| 正文 | 决策级内容（路由、约束、边界） | < 5000 | skill 激活时 |
| 参考文件 | 执行细节（命令签名、响应表、步骤） | 按需 | 用到时 |

规则一句话：**正文写"该做什么决定"，参考文件写"具体怎么做"**。

还有两条更硬的约束：

- **`description` 只写触发条件，不写工作流摘要**。理由：写了摘要，Agent 会照着摘要做而不去读正文。
- **参考文件只允许一层**，不允许 SKILL.md → advanced.md → details.md 这种链式引用。

这两条正好是我们 `skills/worktree-task-flow/SKILL.md` 当前缺的自我约束——它的 description 里带了较长的能力概述。

## 4. CraftingTable 现状

`feat/docs-writing` 分支（未进主分支）已经实现了文档工作区：

| 项 | 现状 | 证据 |
| --- | --- | --- |
| 数据模型 | 单表 `doc_nodes`，`parent_id` 自引用，`kind` 区分 folder/document | `003_docs.sql` |
| 正文 | Markdown 字符串存 `content` 列 | 同上 |
| 检索 | `LIKE` 子串匹配 + 游标分页 | `server/internal/docs/search.go`：`LikePattern`、`EncodeCursor`、`DecodeCursor` |
| 导入导出 | 浏览器 File API，服务端不碰文件系统 | 设计文档非目标第 15 条 |
| 删除 | 软删除 `deleted_at` | `003_docs.sql` |
| 并发 | last-write-wins，无乐观锁 | 设计文档决策摘要 |
| 全文搜索 | **明确列为非目标** | 设计文档 §3.2 第 9 条：「文档数超过 500 或接入 PostgreSQL 全文索引时再做」 |
| 与 Agent 集成 | 不接 | 设计文档 §3.2 第 1 条 |

### 与 MiCO 的能力差距

| 能力 | MiCO | CraftingTable Documents | 差距性质 |
| --- | --- | --- | --- |
| 检索 | 混合（chunk + 蒸馏 + 图谱），RRF 融合 | `LIKE` 子串 | 量级差异 |
| 分块 | `document_chunk` 表，chunk 级召回 | 无 | 结构性缺失 |
| 摘要层 | 蒸馏页（entity/topic） | 无 | 能力缺失 |
| 关系 | 图谱边 + wiki links + 失效链接检测 | 目录树 | 能力缺失 |
| 外部同步 | git/飞书/GitLab/会议，每日自动 | 无 | 需求缺失 |
| 人工纠错 | 五模式 + instruction 留痕 | 无 | 能力缺失 |
| 代码理解 | Atlas 图谱 | 无 | 无关 |
| 权限 | 库成员、admin 角色 | 单用户无权限 | 不需要 |

结论：**差距不是"实现得好不好"，而是"是否需要"**。上表 8 项里，只有"分块检索"和"失效链接检测"对单用户本地应用有明确价值，其余都需要具体场景支撑。

## 5. 触发条件

不设"以后再说"，设可判定条件。任一成立即启动知识库设计与实施。

### K-T1 检索失效（最硬的条件）

当前 `LIKE` 子串检索在以下任一情况成立时不再够用：

| 指标 | 阈值 | 为什么是这条线 |
| --- | --- | --- |
| 文档数 | > 500 | 设计文档已写明的既有判断 |
| 单次检索延迟 | p95 > 300 ms | 全表 `LIKE` 的拐点，需实测确认 |
| 用户需要"意思相近但用词不同"的召回 | 出现一次真实挫败 | 子串检索的结构性缺陷，无法通过调优解决 |

**注意**：前两条可以靠 PostgreSQL 全文索引（`tsvector`）解决，不需要向量检索。第三条才需要向量。

### K-T2 文档需要被 Agent 使用

| 触发 | 说明 |
| --- | --- |
| 用户要求 Agent 回答"我之前记过什么" | 需要检索接口可供 Tool 调用 |
| AG-01 的只读工具扩展到文档域 | 当前只有 `today` 三个工具 |
| AG-09 上下文工程落地 | 需要"检索并注入上下文"的机制 |

这一条与 AG-01/AG-09 联动，不应孤立判断。

### K-T3 内容来源需要自动化

| 触发 | 说明 |
| --- | --- |
| 需要从 Git 仓库 / 外部文档系统持续导入 | 没有这个需求就不该做 `ds-*` 那套 |
| 手工导入频率高到成为负担 | 先测频率，再决定 |

单用户本地应用接外部同步的概率很低。**这条大概率永远不触发，写下来是为了将来不被"别人都有"说服。**

### K-T4 知识需要结构化

| 触发 | 说明 |
| --- | --- |
| 用户需要"这个概念和那个概念是什么关系" | 图谱需求 |
| 需要合并重复条目、纠正错误关联 | 纠错需求 |
| 需要按主题聚合而非按目录浏览 | 蒸馏需求 |

这三条只有在真实使用中出现才成立。**仅凭"更先进"不构成理由。**

### 明确不构成触发条件

| 说法 | 为什么不成立 |
| --- | --- |
| "MiCO 有知识库，我们也应该有" | 产品形态不同：多租户企业协作 vs 单用户本地 |
| "向量检索更先进" | 500 文档以下，子串检索的精确度反而更高，且延迟更低 |
| "以后文档会很多" | 到 500 再切换，迁移成本远低于提前建设的维护成本 |
| "Agent 需要知识库才聪明" | Agent 需要的是**上下文**，不是知识库。AG-09 已经覆盖 |

## 6. 该吸收什么

按"投入产出比"排序，前三条与是否做知识库无关，现在就能做。

### 6.1 作用域锚定（现在就该做）

在 AG-03（工具审批与暂停恢复）设计时，把 MiCO 的五条铁律作为硬约束：

```text
Agent 收到的最窄上下文决定它的作用域上限。
有 selection → 只能改选中片段，不得升级为整篇。
无 selection、有 doc-id → 只能改该文档。
破坏性整体操作 → 必须用户点名 + 二次确认。
```

具体到 CraftingTable，需要两条保障：

1. **工具契约层面**：写工具必须区分"片段级"和"整体级"，不能让"整体级"成为唯一选择。
2. **审批层面**：整体级操作强制走二次确认，不能与片段级共用同一条审批路径。

MiCO 的教训是"skill prose 软约束 + `--force` 硬防护"的纵深防御。我们的对应做法是"工具 Schema 约束 + 审批流程约束"。

### 6.2 原文与摘要双通道（做摘要层的第一天就要有）

如果将来做文档摘要（供 Agent 上下文使用），接口必须同时提供：

- 摘要视图：省 token，适合概览。
- 原文视图：完整内容，适合精确引用。

并且**按请求可切**，而不是建库时定死。MiCO 花了半年才补上 `--source text`，这个坑可以跳过。

### 6.3 渐进式披露（现在就能改）

`skills/worktree-task-flow/SKILL.md` 的 description 目前带了较长概述。按 MiCO 的结论应该改成：

- description 只写触发条件（"什么时候用"），不写"它能做什么"。
- 正文只放决策级内容，命令细节移到 `scripts/` 或 `references/`。

这条改动成本极低，收益是可预期的：description 越像"能力介绍"，Agent 越倾向于照它做事而不读正文。

### 6.4 分块检索（条件触发后做）

当 K-T1 的前两条触发（文档数 > 500 或延迟超标）时：

1. 先上 PostgreSQL `tsvector` + `pg_trgm`，不引入向量。
2. 数据模型增加 `doc_chunks` 表：`document_id`、`chunk_index`、`content`、`tokens`、`tsv`。
3. 检索返回 chunk 级结果，带 `documentId` + `chunkId` + `snippet` + `body`，与 MiCO 的 `SearchResult` 同形。

在第 1 步就能解决问题的情况下引入向量，是负收益。

### 6.5 失效链接检测（低成本，可提前做）

MiCO 的 `doc-dead-links` 是一个独立小功能：列出文档里指向不存在目标的链接。

对 CraftingTable，等 Documents 工作区进主分支后，可以做一个更简单的版本：扫描 `doc_nodes.content` 里的内部链接，比对目标是否存在。**成本约一个人天，收益是不可见的数据腐坏变成可见清单。**

## 7. 明确不该吸收

| 不吸收 | 理由 |
| --- | --- |
| 蒸馏页（entity/topic） | 需要 LLM 定期跑批，单用户场景收益低于成本 |
| 知识图谱边 + 五种纠错模式 | 无多用户协作时，"纠正别人的错误关联"这个需求不存在 |
| 22 个 CLI 子命令 | 我们的 API 面应随需求增长，不预设 |
| 外部数据源同步（git/飞书/会议） | 单用户本地应用，`local_file` 之外的来源概率极低 |
| Atlas 代码图谱 | 与产品目标无关（那是给 Agent 读代码用的） |
| 库成员与 admin 角色 | 单用户无权限模型 |
| 实时协同编辑 | 设计已明确非目标 |
| `--output agent` 格式 | 这个值得借鉴，但当前阶段没有 Agent 消费方，先不做 |

最后一条要说明：`--output agent`（紧凑 JSON，去掉冗余预览）是很聪明的设计，但**在没有消费方时实现它是纯粹的浪费**。等 AG-09 落地时再补。

## 8. 如果要做，数据模型方向

仅在 §5 触发条件成立时启用。方向性草案，不是设计。

```text
doc_nodes           现有表保持不动（目录树 + Markdown 正文）
doc_chunks          新增：分块与索引
  id, document_id, chunk_index, content, token_count, tsv
  UNIQUE(document_id, chunk_index)

doc_links           新增：内部链接（支撑失效检测与反链）
  id, source_document_id, target_document_id, target_kind, created_at
  UNIQUE(source_document_id, target_document_id, target_kind)
```

三条约束：

1. **单表自引用不要拆**。MiCO 用 `doc_nodes` 单表承载目录与文档是可行的，我们的 `003_docs.sql` 已经是这个形状，不要为了"规范化"拆成两张。
2. **chunk 是派生数据**，可以从 `doc_nodes.content` 重建。因此它不需要迁移历史，重建成本可接受。
3. **不建 `pages` 表**。摘要层在触发前不实现；实现时优先考虑"按需生成 + 缓存"而不是"定期全量蒸馏"。

## 9. 风险

| 风险 | 表现 | 缓解 |
| --- | --- | --- |
| 在未验收的 Documents 之上建知识库 | 两个不稳定层叠加，问题无法定位 | 先让 Documents 进主分支并有测试基线（Q-05） |
| 过早引入向量检索 | 延迟上升、结果不可解释、调优成本高 | 先用 PostgreSQL 全文索引，向量只在 K-T1 第三条触发时引入 |
| 摘要层与原文不一致 | Agent 引用摘要回答，但原文已变 | 摘要带 `sourceUpdatedAt`，不一致时降级为原文 |
| Agent 越权修改 | 与 MiCO ADR-0012 同类事故 | §6.1 的锚定规则 + 审批分层 |
| 检索接口过早固化 | 将来接向量时需要破坏性变更 | 沿用 MiCO 的 `source` override 思路，模式可按请求覆盖 |

## 10. 待决策

| 编号 | 问题 | 备选 |
| --- | --- | --- |
| KQ-01 | Documents 工作区何时进主分支 | 先完成 Q-05 测试基线 / 直接合并 / 重做 |
| KQ-02 | 检索先用 `LIKE` 还是直接上 `tsvector` | 保持 `LIKE` / 直接 `tsvector` / 两者并存可切 |
| KQ-03 | 是否需要文档摘要层 | 需要 / 不需要 / 等 Agent 上下文需求明确 |
| KQ-04 | Agent 写文档的审批粒度 | 按操作类型 / 按文档 / 全量二次确认 |
| KQ-05 | 是否现在就把 skill description 改成纯触发条件 | 现在改 / 等下次迭代 |
| KQ-06 | 失效链接检测是否提前做 | 提前 / 随知识库一起 / 不做 |

## 11. 参考

MiCO 仓库（`~/repos/mico`，`main` @ `fb894a8f639`）：

- `docs/wiki-subapp-dev-guide.md` — 四仓库分工、端口、启动与发布流程
- `docs/mico/knowledge-decoupling-freeze.md` — 知识业务迁出的冻结边界、声明分类与基线
- `docs/superpowers/specs/2026-09-16-mico-wiki-search-raw-text-flag-design.md` — `--source text` 的设计与背景
- `docs/superpowers/specs/2026-09-16-mico-wiki-skill-progressive-disclosure-design.md` — 三层渐进式披露
- `docs/adr/0012-knowledge-context-scope-anchoring.md` — 作用域锚定五条铁律与事故复盘
- `docs/mico/atlas-distill.md` — 蒸馏报告推送链路与跨仓分工
- `server/internal/mico/knowledge/models.go` — `SearchRequest` / `SearchResult` / `CreateLibraryRequest` 契约
- `server/internal/mico/knowledgeplatform/http_handler.go` — BFF 层与 Java 服务的协议适配
- `server/internal/mico/knowledge/sync_service.go` — 成员同步（含外部成员的两个陷阱）

CraftingTable 仓库：

- `feat/docs-writing` 分支 `docs/superpowers/specs/2026-11-05-craftingtable-documents-design.md`
- `feat/docs-writing` 分支 `server/internal/platform/store/migrations/003_docs.sql`
- [测试策略调研](./2026-11-06-testing-strategy.md)
- [通用 Agent 调研](./2026-11-06-general-agent-comparison.md)
