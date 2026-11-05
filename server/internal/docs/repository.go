package docs

// 本文件保留 Repository 端口的位置说明。
// 端口声明位于 node.go，与领域类型同处一个包以便实现方直接引用。
//
// 约定：
//   1. GetNode、RenameNode、MoveNode、SaveContent、SoftDelete 对已软删除节点返回 ErrNodeNotFound。
//   2. SoftDelete 必须在单个事务内递归标记子孙，并返回全部受影响 id。
//   3. Search 的大小写不敏感语义由实现保证：SQLite 用 LIKE，其他实现可用 ILIKE。
//   4. Search 内部取 limit+1 条判定是否有下一页，返回给上层的是截断后的结果与 nextCursor。
//   5. AncestorIDs 返回从直接父节点到根的 id 列表，不包含自身。
