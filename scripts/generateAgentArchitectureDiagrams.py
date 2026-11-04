from __future__ import annotations

from html import escape
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
OUTPUT_DIR = ROOT / "docs" / "architecture"

COLORS = {
    "canvas": "#FFFFFF",
    "text": "#111827",
    "muted": "#6B7280",
    "border": "#D1D5DB",
    "blue": "#2563EB",
    "blue_soft": "#EFF6FF",
    "blue_border": "#BFDBFE",
    "green": "#16A34A",
    "green_soft": "#F0FDF4",
    "green_border": "#BBF7D0",
    "purple": "#9333EA",
    "purple_soft": "#FAF5FF",
    "purple_border": "#E9D5FF",
    "orange": "#EA580C",
    "orange_soft": "#FFF7ED",
    "orange_border": "#FED7AA",
    "red": "#DC2626",
    "red_soft": "#FEF2F2",
    "gray": "#64748B",
    "gray_soft": "#F8FAFC",
    "teal": "#0F766E",
    "teal_soft": "#F0FDFA",
}

ACCENTS = {
    "blue": (COLORS["blue"], COLORS["blue_soft"], COLORS["blue_border"]),
    "green": (COLORS["green"], COLORS["green_soft"], COLORS["green_border"]),
    "purple": (COLORS["purple"], COLORS["purple_soft"], COLORS["purple_border"]),
    "orange": (COLORS["orange"], COLORS["orange_soft"], COLORS["orange_border"]),
    "red": (COLORS["red"], COLORS["red_soft"], "#FECACA"),
    "gray": ("#64748B", COLORS["gray_soft"], "#E2E8F0"),
    "teal": (COLORS["teal"], COLORS["teal_soft"], "#99F6E4"),
}


def text(
    x: float,
    y: float,
    value: str,
    *,
    size: int = 14,
    weight: int = 400,
    fill: str = COLORS["text"],
    anchor: str = "start",
    owner: str | None = None,
) -> str:
    owner_attr = f' data-owner="{escape(owner, quote=True)}"' if owner else ""
    return (
        f'<text x="{x}" y="{y}" font-size="{size}" font-weight="{weight}" '
        f'fill="{fill}" text-anchor="{anchor}"{owner_attr}>{escape(value)}</text>'
    )


def background(width: int, height: int) -> str:
    return (
        f'<rect x="0" y="0" width="{width}" height="{height}" '
        f'fill="{COLORS["canvas"]}" data-graph-role="background"/>'
    )


def defs() -> str:
    return """
  <defs>
    <marker id="arrow-blue" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
      <polygon points="0 0, 10 3.5, 0 7" fill="#2563EB"/>
    </marker>
    <marker id="arrow-green" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
      <polygon points="0 0, 10 3.5, 0 7" fill="#16A34A"/>
    </marker>
    <marker id="arrow-purple" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
      <polygon points="0 0, 10 3.5, 0 7" fill="#9333EA"/>
    </marker>
    <marker id="arrow-orange" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
      <polygon points="0 0, 10 3.5, 0 7" fill="#EA580C"/>
    </marker>
    <marker id="arrow-red" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
      <polygon points="0 0, 10 3.5, 0 7" fill="#DC2626"/>
    </marker>
    <marker id="arrow-gray" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
      <polygon points="0 0, 10 3.5, 0 7" fill="#64748B"/>
    </marker>
  </defs>
""".strip()


def shell(width: int, height: int, title: str, subtitle: str) -> list[str]:
    return [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" '
        f'viewBox="0 0 {width} {height}" data-quality-profile="standard">',
        "  <style>",
        "    text { font-family: 'Helvetica Neue', Helvetica, Arial, 'PingFang SC', 'Microsoft YaHei', 'Microsoft JhengHei', 'SimHei', sans-serif; }",
        "  </style>",
        defs(),
        background(width, height),
        text(48, 42, title, size=22, weight=600),
        text(48, 67, subtitle, size=13, fill=COLORS["muted"]),
    ]


def container(
    x: int,
    y: int,
    width: int,
    height: int,
    label: str,
    *,
    accent: str = "gray",
    dashed: bool = False,
) -> list[str]:
    color, tint, border = ACCENTS[accent]
    dash = ' stroke-dasharray="7 5"' if dashed else ""
    return [
        f'<rect id="{escape(label, quote=True)}" x="{x}" y="{y}" width="{width}" height="{height}" '
        f'rx="12" fill="{tint}" fill-opacity="0.32" stroke="{border}" stroke-width="1.25"{dash} '
        f'data-graph-role="container"/>',
        text(x + 18, y + 24, label, size=13, weight=600, fill=color),
        f'<line x1="{x + 16}" y1="{y + 36}" x2="{x + width - 16}" y2="{y + 36}" '
        f'stroke="{border}" stroke-width="1"/>',
    ]


def node(
    node_id: str,
    x: int,
    y: int,
    width: int,
    height: int,
    title: str,
    subtitle: str = "",
    *,
    accent: str = "blue",
    dashed: bool = False,
    badge: str = "",
) -> list[str]:
    color, tint, border = ACCENTS[accent]
    dash = ' stroke-dasharray="6 4"' if dashed else ""
    lines = [
        f'<g id="g-{escape(node_id, quote=True)}">',
        f'<rect id="{escape(node_id, quote=True)}" data-node-id="{escape(node_id, quote=True)}" '
        f'data-graph-role="node" x="{x}" y="{y}" width="{width}" height="{height}" rx="8" '
        f'fill="{COLORS["canvas"]}" stroke="{border}" stroke-width="1.5"{dash}/>',
        f'<rect x="{x + 10}" y="{y + 12}" width="26" height="26" rx="6" fill="{tint}" stroke="{border}"/>',
    ]
    if badge:
        lines.append(text(x + 23, y + 30, badge, size=11, weight=600, fill=color, anchor="middle"))
    title_x = x + 48 if badge else x + 18
    lines.append(text(title_x, y + 29, title, size=14, weight=600))
    if subtitle:
        for index, subtitle_line in enumerate(subtitle.split("\n")):
            lines.append(text(x + 18, y + 51 + index * 18, subtitle_line, size=11.5, fill=COLORS["muted"]))
    lines.append("</g>")
    return lines


def edge(
    edge_id: str,
    d: str,
    color: str = "blue",
    *,
    dashed: bool = False,
    width: float = 1.7,
) -> list[str]:
    dash = ' stroke-dasharray="5 4"' if dashed else ""
    return [(
        f'<path id="{escape(edge_id, quote=True)}" data-edge-id="{escape(edge_id, quote=True)}" '
        f'data-graph-role="edge" d="{d}" fill="none" stroke="{COLORS[color]}" '
        f'stroke-width="{width}"{dash} marker-end="url(#arrow-{color})"/>'
    )]


def label(
    x: int,
    y: int,
    value: str,
    *,
    anchor: str = "middle",
    owner: str = "",
    fill: str = COLORS["muted"],
    size: float = 11.5,
    padded: bool = True,
) -> list[str]:
    if not padded:
        return [text(x, y, value, size=size, fill=fill, anchor=anchor, owner=owner)]
    estimated = max(42, len(value) * size * 0.78 + 12)
    if anchor == "middle":
        left = x - estimated / 2
    elif anchor == "end":
        left = x - estimated
    else:
        left = x
    return [
        f'<g data-owner="{escape(owner, quote=True)}">',
        f'<rect x="{left:.1f}" y="{y - 13}" width="{estimated:.1f}" height="18" rx="4" '
        f'fill="#FFFFFF" fill-opacity="0.94"/>',
        text(x, y, value, size=size, fill=fill, anchor=anchor, owner=owner),
        "</g>",
    ]


def legend(items: list[tuple[str, str, bool]], x: int, y: int) -> list[str]:
    lines = [f'<g id="legend" data-graph-role="legend">']
    cursor = x
    for color, caption, dashed in items:
        dash = ' stroke-dasharray="5 4"' if dashed else ""
        lines.append(
            f'<line x1="{cursor}" y1="{y}" x2="{cursor + 28}" y2="{y}" stroke="{COLORS[color]}" '
            f'stroke-width="1.7"{dash} marker-end="url(#arrow-{color})"/>'
        )
        lines.append(text(cursor + 36, y + 4, caption, size=11.5, fill=COLORS["muted"]))
        cursor += 142
    lines.append("</g>")
    return lines


def footer_note(width: int, y: int, value: str) -> list[str]:
    return [
        f'<rect x="48" y="{y}" width="{width - 96}" height="34" rx="7" '
        f'fill="{COLORS["gray_soft"]}" stroke="#E5E7EB"/>',
        text(64, y + 22, value, size=11.5, fill=COLORS["muted"]),
    ]


def architecture_diagram() -> list[str]:
    width, height = 1200, 960
    lines = shell(
        width,
        height,
        "CraftingTable Agent 系统架构",
        "单 Agent 最小闭环；Access、Application、Core 与 Adapters 依赖方向固定",
    )
    lines += container(40, 88, 1120, 222, "React Renderer  ·  Desktop / Web 共用 MVC", accent="blue")
    lines += container(40, 342, 1120, 84, "平台传输边界", accent="orange")
    lines += container(40, 458, 1120, 248, "Go 模块化单体  ·  Agent 分层", accent="green")
    lines += container(40, 738, 1120, 128, "持久化与可观测边界", accent="purple")

    lines += node("view", 82, 136, 220, 74, "View", "消息 / 会话侧栏 / 输入区", accent="blue", badge="V")
    lines += node("effect", 360, 136, 220, 74, "AgentEffect", "请求生命周期与流订阅", accent="orange", badge="E")
    lines += node("model", 638, 136, 220, 74, "AgentModel", "Transport + 流映射", accent="orange", badge="M")
    lines += node("store", 916, 136, 202, 74, "Zustand Store", "前端唯一业务事实源", accent="green", badge="S")
    lines += node("draft-store", 360, 232, 220, 58, "同步 action", "只写可序列化状态", accent="green", badge="")

    lines += edge("view-action", "M302,173 L360,173", "blue")
    lines += label(331, 164, "动作", owner="view-action", padded=False)
    lines += edge("effect-model", "M580,173 L638,173", "orange")
    lines += label(609, 164, "调用", owner="effect-model", padded=False)
    lines += edge("effect-store", "M470,210 L470,232", "green")
    lines += label(492, 226, "写入", owner="effect-store", padded=False)
    lines += edge("store-view", "M360,257 L112,257 L112,210", "green")
    lines += label(236, 249, "读取 / selector", owner="store-view", padded=False)

    lines += node(
        "fetch-adapter",
        394,
        360,
        412,
        48,
        "Platform Fetch Adapter",
        "Web: fetch  ·  Desktop: MessagePort",
        accent="orange",
        badge="T",
    )
    lines += edge("model-transport", "M748,210 L748,360", "orange")
    lines += label(768, 286, "HTTP + SSE", owner="model-transport", padded=False)

    lines += node("http-controller", 74, 508, 212, 78, "HTTP Controller", "解码 / 校验 / SSE", accent="green", badge="H")
    lines += node("app-controller", 328, 508, 212, 78, "Application Controller", "用例编排与幂等入口", accent="green", badge="A")
    lines += node("coordinator", 582, 508, 212, 78, "Run Coordinator", "会话槽 / runCtx / 取消", accent="green", badge="C")
    lines += node("executor", 836, 508, 212, 78, "Agent Executor", "循环、策略、终态收敛", accent="purple", badge="X")
    lines += node("runtime", 582, 628, 212, 56, "Agent Runtime", "单次供应商调用", accent="purple", badge="R")
    lines += node("provider", 836, 628, 212, 56, "Provider Adapter", "原始协议 → 领域事件", accent="purple", badge="P")

    lines += edge("fetch-http", "M600,408 L600,438 L180,438 L180,508", "orange")
    lines += label(386, 430, "POST /api/v1/agent/messages", owner="fetch-http", padded=False)
    lines += edge("http-app", "M286,547 L328,547", "green")
    lines += edge("app-coordinator", "M540,547 L582,547", "green")
    lines += edge("coordinator-executor", "M794,547 L836,547", "green")
    lines += edge("executor-runtime", "M942,586 L942,608 L794,608 L794,628", "purple")
    lines += edge("runtime-provider", "M794,656 L836,656", "purple")
    lines += label(815, 647, "调用", owner="runtime-provider", padded=False)

    lines += node("repository", 286, 774, 292, 62, "Agent Repository", "Session / Message / Run / Event", accent="purple", badge="D")
    lines += node("database", 646, 774, 292, 62, "SQLite / PostgreSQL", "Part 存正文；Event 存生命周期", accent="purple", badge="DB")
    lines += edge("coordinator-repository", "M688,586 L688,600 L560,600 L560,716 L432,716 L432,774", "purple", dashed=True)
    lines += label(561, 707, "事务与查询", owner="coordinator-repository", padded=False)
    lines += edge("repository-database", "M578,805 L646,805", "purple")
    lines += label(
        712,
        856,
        "运行态：Run 独立于 HTTP 连接；Zustand 不保存 Stream、AbortController 或数据库实体",
        owner="",
        padded=False,
        size=11.5,
    )

    lines += legend(
        [
            ("blue", "用户动作", False),
            ("orange", "调用 / 传输", False),
            ("green", "状态与编排", False),
            ("purple", "执行与持久化", False),
        ],
        64,
        914,
    )
    lines.append("</svg>")
    return lines


def layered_architecture_diagram() -> list[str]:
    width, height = 1200, 1040
    lines = shell(
        width,
        height,
        "CraftingTable Agent 分层与依赖方向",
        "参考职责分层，不照搬包结构；Core 拥有端口，Adapter 实现端口",
    )
    lines += container(40, 88, 1120, 180, "体验层  ·  Experience", accent="blue")
    lines += container(40, 292, 1120, 116, "接入层  ·  Access", accent="orange")
    lines += container(40, 432, 1120, 136, "Agent 应用层  ·  Application", accent="green")
    lines += container(40, 592, 1120, 164, "Agent 核心层  ·  Core", accent="purple")
    lines += container(40, 780, 1120, 162, "适配器与基础设施  ·  Adapters / Infrastructure", accent="gray")

    lines += node("layer-view", 80, 130, 210, 80, "View", "只渲染与触发动作", accent="blue", badge="V")
    lines += node("layer-effect", 320, 130, 210, 80, "AgentEffect", "异步流程 / 流订阅", accent="orange", badge="E")
    lines += node("layer-model", 560, 130, 210, 80, "AgentModel", "契约 / Transport / 映射", accent="orange", badge="M")
    lines += node("layer-store", 820, 130, 250, 80, "Zustand Store", "唯一可恢复业务状态", accent="green", badge="S")

    lines += edge("layer-view-effect", "M290,170 L320,170", "blue")
    lines += edge("layer-effect-model", "M530,170 L560,170", "orange")

    lines += node("layer-http", 160, 326, 340, 64, "HTTP Controller", "解码 / 校验 / 响应", accent="green", badge="H")
    lines += node("layer-stream-view", 620, 326, 340, 64, "Stream View", "RunEvent → UI Stream SSE", accent="blue", badge="V")
    lines += edge("layer-http-stream", "M500,358 L620,358", "blue")
    lines += edge("layer-model-http", "M665,210 L665,278 L330,278 L330,326", "orange")
    lines += label(500, 270, "HTTP + SSE", owner="layer-model-http", padded=False)

    lines += node("layer-app", 100, 476, 290, 64, "Application Controller", "用例编排 / 幂等入口", accent="green", badge="A")
    lines += node("layer-coordinator", 470, 476, 260, 64, "Run Coordinator", "执行槽 / runCtx / 取消", accent="green", badge="C")
    lines += node("layer-session-slot", 800, 476, 280, 64, "Session Slot + Idempotency", "一次 Run 的并发与重放保护", accent="green", badge="I")
    lines += edge("layer-http-app", "M330,390 L330,476", "green")
    lines += label(350, 425, "进入用例边界", owner="layer-http-app", padded=False, anchor="start")
    lines += edge("layer-app-coordinator", "M390,508 L470,508", "green")
    lines += edge("layer-coordinator-slot", "M730,508 L800,508", "green")

    lines += node("layer-executor", 80, 640, 260, 76, "AgentExecutor", "Agent Loop / 终态收敛", accent="purple", badge="X")
    lines += node("layer-state", 380, 640, 250, 76, "RunPolicy + StateMachine", "Step 边界 / 超时 / 失败上限", accent="purple", badge="R")
    lines += node("layer-repository-port", 670, 640, 200, 76, "RepositoryPort", "Core-owned abstraction", accent="purple", badge="D")
    lines += node("layer-runtime-port", 910, 640, 210, 76, "RuntimePort", "Core-owned abstraction", accent="purple", badge="R")
    lines += edge("layer-coordinator-executor", "M610,540 L610,580 L225,580 L225,640", "purple")
    lines += label(418, 572, "启动 runCtx", owner="layer-coordinator-executor", padded=False)
    lines += edge("layer-executor-state", "M340,678 L380,678", "purple")

    lines += node("layer-repository-adapter", 200, 820, 280, 70, "Repository Adapter", "Session / Message / Run / Event", accent="gray", badge="D")
    lines += node("layer-database", 520, 820, 240, 70, "SQLite / PostgreSQL", "Desktop / Web 存储适配", accent="gray", badge="DB")
    lines += node("layer-provider-adapter", 810, 820, 280, 70, "Provider Adapter", "OpenAI-compatible 流适配", accent="gray", badge="P")
    lines += edge("layer-repository-database", "M480,855 L520,855", "gray")
    lines += edge("layer-repository-port-impl", "M340,820 L340,768 L770,768 L770,716", "gray", dashed=True)
    lines += label(558, 760, "实现 RepositoryPort", owner="layer-repository-port-impl", padded=False)
    lines += edge("layer-runtime-port-impl", "M950,820 L950,744 L1015,744 L1015,716", "gray", dashed=True)
    lines += label(980, 736, "实现 RuntimePort", owner="layer-runtime-port-impl", padded=False)

    lines += footer_note(
        width,
        958,
        "允许依赖：Experience → Access → Application → Core；Adapter 向内实现 Core 端口。禁止 Core 导入 HTTP、SQLite 或 Provider SDK，依赖由 bootstrap 注入。",
    )
    lines += legend(
        [
            ("blue", "展示与流回传", False),
            ("orange", "入口与传输", False),
            ("green", "应用用例", False),
            ("purple", "Core 与端口", False),
            ("gray", "适配器实现端口", True),
        ],
        64,
        1012,
    )
    lines.append("</svg>")
    return lines


def sequence_diagram() -> list[str]:
    width, height = 1200, 1040
    lines = shell(
        width,
        height,
        "CraftingTable Agent Run 时序",
        "发送消息、获取会话执行槽、流式执行、持久化终态与释放槽位",
    )
    participants = [
        (90, "Effect", "发起与收尾", "orange"),
        (240, "Model", "Transport", "orange"),
        (390, "HTTP", "API Controller", "green"),
        (540, "Coordinator", "获得执行槽", "green"),
        (690, "Executor", "Run 状态机", "purple"),
        (840, "Runtime", "单步模型调用", "purple"),
        (990, "Repository", "事务持久化", "blue"),
        (1130, "StreamView", "SSE 编码", "blue"),
    ]
    for x, title, subtitle, accent in participants:
        color, tint, border = ACCENTS[accent]
        lines.append(
            f'<rect data-graph-role="node" data-node-id="participant-{escape(title, quote=True)}" '
            f'x="{x - 61}" y="92" width="122" height="54" rx="8" fill="{COLORS["canvas"]}" '
            f'stroke="{border}" stroke-width="1.5"/>'
        )
        lines.append(f'<circle cx="{x - 42}" cy="119" r="10" fill="{tint}" stroke="{border}"/>')
        lines.append(text(x, 116, title, size=13, weight=600, anchor="middle", fill=color))
        lines.append(text(x, 136, subtitle, size=10.5, anchor="middle", fill=COLORS["muted"]))
        lines.append(
            f'<line x1="{x}" y1="146" x2="{x}" y2="984" stroke="#CBD5E1" '
            f'stroke-width="1.2" stroke-dasharray="4 5" data-graph-role="decoration"/>'
        )

    def msg(
        idx: int,
        source: int,
        target: int,
        value: str,
        *,
        color: str = "blue",
        dashed: bool = False,
        y: int | None = None,
    ) -> int:
        source_x = participants[source][0]
        target_x = participants[target][0]
        current_y = y if y is not None else 190 + idx * 43
        direction = 1 if target_x > source_x else -1
        start = source_x + direction * 8
        end = target_x - direction * 8
        dash = ' stroke-dasharray="5 4"' if dashed else ""
        owner = f"seq-{idx}"
        lines.append(
            f'<path id="{owner}" data-edge-id="{owner}" data-graph-role="edge" '
            f'd="M{start},{current_y} L{end},{current_y}" fill="none" stroke="{COLORS[color]}" '
            f'stroke-width="1.6"{dash} marker-end="url(#arrow-{color})"/>'
        )
        lines.append(
            text(
                (source_x + target_x) / 2,
                current_y - 7,
                value,
                size=10.8,
                fill=COLORS["text"],
                anchor="middle",
                owner=owner,
            )
        )
        return current_y

    def frame(start_y: int, end_y: int, title_value: str, first_message_y: int) -> None:
        lines.append(
            f'<rect x="48" y="{start_y}" width="1104" height="{end_y - start_y}" rx="8" '
            f'fill="none" stroke="#CBD5E1" stroke-width="1.2" data-graph-role="container"/>'
        )
        badge_y = first_message_y - 11
        lines.append(
            f'<rect x="62" y="{badge_y}" width="{max(122, len(title_value) * 13 + 20)}" height="22" '
            f'rx="5" fill="#F8FAFC" stroke="#CBD5E1"/>'
        )
        lines.append(text(74, badge_y + 15, title_value, size=11.5, weight=600, fill="#475569"))

    frame(174, 390, "01 接收与抢占", 202)
    frame(408, 804, "02 Executor 驱动 Runtime 并发布流事件", 439)
    frame(822, 966, "03 提交终态并释放执行槽", 853)

    msg(0, 0, 1, "send(sessionId, requestHash)", color="orange")
    msg(1, 1, 2, "POST message + clientRequestId", color="orange")
    msg(2, 2, 3, "acquireSessionSlot()", color="green")
    msg(3, 3, 6, "查幂等记录 / 开启事务", color="green")
    msg(4, 6, 3, "existingRun 或 slotGranted", color="green", dashed=True)
    msg(5, 3, 4, "start(runCtx, RunPolicy)", color="green")

    msg(6, 4, 6, "创建 MessagePart / Run / Step", color="blue")
    msg(7, 6, 4, "事务提交", color="blue", dashed=True)
    msg(8, 4, 5, "Runtime.Stream(runCtx)", color="purple")
    msg(9, 5, 7, "RunDelta / RunEvent", color="purple")
    msg(10, 7, 1, "UI Message Stream chunks", color="blue")
    msg(11, 1, 0, "onStarted / onSnapshot", color="blue", dashed=True)
    msg(12, 5, 4, "finish 或 stable error", color="purple", dashed=True)

    msg(13, 4, 6, "终态事务：Part 正文 + Event", color="green")
    msg(14, 6, 4, "commit ok", color="green", dashed=True)
    msg(15, 4, 7, "publish finish", color="purple")
    msg(16, 7, 1, "finish / [DONE]", color="blue")
    msg(17, 4, 3, "releaseSessionSlot()", color="red")
    msg(18, 1, 0, "Result<AgentSendResult>", color="green", dashed=True)

    lines += legend(
        [
            ("orange", "请求与传输", False),
            ("green", "编排与提交", False),
            ("purple", "模型执行", False),
            ("blue", "流式回传", False),
            ("red", "释放资源", False),
        ],
        64,
        1008,
    )
    lines.append("</svg>")
    return lines


def state_diagram() -> list[str]:
    width, height = 1200, 780
    lines = shell(
        width,
        height,
        "CraftingTable Agent 状态机",
        "Run 与 Step 都只有一个运行态和唯一终态；取消、失败、中断不能只是停止 goroutine",
    )
    lines += container(44, 92, 1112, 282, "Run 生命周期", accent="blue")
    lines += container(44, 402, 1112, 292, "Step 生命周期与 Executor 循环", accent="purple")

    lines.append('<circle cx="100" cy="222" r="8" fill="#111827" data-graph-role="decoration"/>')
    lines += node("run-running", 198, 186, 210, 72, "running", "Executor 持有 runCtx", accent="blue", badge="R")
    lines += node("run-completed", 486, 136, 176, 68, "completed", "最终答案已提交", accent="green", badge="C")
    lines += node("run-failed", 746, 136, 176, 68, "failed", "稳定错误码", accent="red", badge="F")
    lines += node("run-cancelled", 486, 260, 176, 68, "cancelled", "用户主动停止", accent="orange", badge="X")
    lines += node("run-interrupted", 746, 260, 176, 68, "interrupted", "进程重启清理", accent="gray", badge="I")
    lines += edge("init-run", "M108,222 L198,222", "gray")
    lines += edge("run-complete", "M408,196 L486,196", "green")
    lines += label(447, 188, "finish", owner="run-complete", padded=False)
    lines += edge("run-fail", "M350,258 L350,370 L948,370 L948,170 L922,170", "red")
    lines += label(366, 286, "provider / policy error", owner="run-fail", padded=False, anchor="start")
    lines += edge("run-cancel", "M408,240 L486,240 L486,260", "orange")
    lines += label(454, 232, "cancel", owner="run-cancel", padded=False)
    lines += edge("run-interrupt", "M408,258 L438,258 L438,358 L822,358 L822,328", "gray", dashed=True)
    lines += label(650, 350, "startup reconciliation", owner="run-interrupt", padded=False)

    lines.append('<circle cx="100" cy="552" r="8" fill="#111827" data-graph-role="decoration"/>')
    lines += node("step-running", 198, 516, 210, 72, "step.running", "一次 model / tool / MCP", accent="purple", badge="S")
    lines += node("step-completed", 470, 474, 176, 68, "completed", "有界操作成功", accent="green", badge="C")
    lines += node("step-failed", 710, 474, 176, 68, "failed", "记录失败原因", accent="red", badge="F")
    lines += node("step-cancelled", 470, 602, 176, 68, "cancelled", "runCtx 取消", accent="orange", badge="X")
    lines += node("step-interrupted", 710, 602, 176, 68, "interrupted", "进程恢复扫描", accent="gray", badge="I")
    lines += node("step-exhausted", 944, 474, 176, 68, "Run failed", "maxSteps 耗尽", accent="red", badge="!")
    lines.append('<circle cx="1028" cy="594" r="8" fill="#111827" data-graph-role="decoration"/>')
    lines.append('<circle cx="1028" cy="594" r="13" fill="none" stroke="#111827" stroke-width="2" data-graph-role="decoration"/>')

    lines += edge("init-step", "M108,552 L198,552", "gray")
    lines += edge("step-finish", "M408,540 L438,540 L438,508 L470,508", "green")
    lines += label(440, 532, "success", owner="step-finish", padded=False)
    lines += edge("step-fail", "M330,588 L330,690 L926,690 L926,508 L886,508", "red")
    lines += label(366, 632, "error", owner="step-fail", padded=False, anchor="start")
    lines += edge("step-cancel", "M408,574 L470,574 L470,602", "orange")
    lines += label(438, 571, "cancel", owner="step-cancel", padded=False)
    lines += edge("step-interrupt", "M408,588 L438,588 L438,686 L798,686 L798,670", "gray", dashed=True)
    lines += label(540, 678, "crash scan", owner="step-interrupt", padded=False)
    lines += edge("step-next", "M558,474 L558,444 L296,444 L296,516", "purple", dashed=True)
    lines += label(430, 436, "无最终答案且未达 maxSteps → 下一 Step", owner="step-next", padded=False)
    lines += edge("step-limit", "M646,508 L710,508", "red", dashed=True)
    lines += label(678, 500, "last step", owner="step-limit", padded=False)
    lines += edge("run-final-failed", "M798,474 L798,444 L1030,444 L1030,474", "red", dashed=True)
    lines += edge("step-final", "M974,542 L988,542 L988,594", "gray", dashed=True)

    lines += footer_note(
        width,
        720,
        "RunPolicy：maxSteps · maxDurationMs · idleTimeoutMs · maxConsecutiveFailures；所有限制都由 Executor 检查并收敛到唯一终态。",
    )
    lines += legend(
        [
            ("green", "成功", False),
            ("red", "失败", False),
            ("orange", "取消", False),
            ("gray", "中断", True),
        ],
        64,
        772,
    )
    lines.append("</svg>")
    return lines


def data_model_diagram() -> list[str]:
    width, height = 1200, 940
    lines = shell(
        width,
        height,
        "CraftingTable Agent 数据模型",
        "Part 是可展示正文的唯一持久化来源；RunEvent 只记录语义生命周期",
    )
    lines += container(40, 92, 1120, 198, "消息与展示片段", accent="blue")
    lines += container(40, 324, 1120, 214, "执行与生命周期", accent="purple")
    lines += container(40, 572, 1120, 210, "幂等、事件与未来 Artifact", accent="green")

    lines += node("session", 72, 134, 250, 132, "AgentSession", "id (PK) · title\ncreatedAt · updatedAt", accent="blue", badge="S")
    lines += node("message", 380, 134, 278, 132, "AgentMessage", "id (PK) · sessionId (FK)\nseq · role · status", accent="blue", badge="M")
    lines += node("part", 738, 134, 330, 132, "AgentMessagePart", "id (PK) · messageId (FK) · runId\nindex · type · status\ncontent: text", accent="blue", badge="P")

    lines += node("idempotency", 72, 374, 250, 132, "IdempotencyKey", "clientRequestId (PK)\nrequestHash · runId\nstatus", accent="green", badge="I")
    lines += node("run", 380, 374, 288, 132, "AgentRun", "id (PK) · sessionId (FK)\nuserMessageId · assistantMessageId\nstatus · policyVersion", accent="purple", badge="R")
    lines += node("step", 738, 374, 330, 132, "AgentStep", "id (PK) · runId (FK) · index\nkind · status\nstartedAt · finishedAt", accent="purple", badge="X")

    lines += node("event", 72, 626, 286, 118, "AgentRunEvent", "id (PK) · runId (FK)\nsequence · type · payload", accent="green", badge="E")
    lines += node("artifact", 722, 626, 290, 118, "Artifact  ·  未来扩展", "file / image / tool log\nrunId or stepId reference", accent="gray", dashed=True, badge="A")
    lines += footer_note(
        width,
        814,
        "关系不变量：Session 1:N Message；Message 1:N Part；Run 1:N Step / Event；Part 的生成片段必须带 runId；用户消息 Part 不带 runId。",
    )

    lines += edge("session-message", "M322,200 L380,200", "blue")
    lines += label(351, 191, "1 : N", owner="session-message", padded=False)
    lines += edge("message-part", "M658,200 L738,200", "blue")
    lines += label(698, 191, "1 : N", owner="message-part", padded=False)
    lines += edge("message-run", "M519,266 L519,320 L454,320 L454,374", "purple", dashed=True)
    lines += label(486, 312, "触发 / 回答", owner="message-run", padded=False)
    lines += edge("session-run", "M197,266 L197,312 L330,312 L330,374 L380,374", "purple", dashed=True)
    lines += label(262, 304, "1 : N", owner="session-run", padded=False)
    lines += edge("run-step", "M668,440 L738,440", "purple")
    lines += label(703, 431, "1 : N", owner="run-step", padded=False)
    lines += edge("run-part", "M668,392 L704,392 L704,266", "blue", dashed=True)
    lines += label(706, 338, "runId", owner="run-part", padded=False)
    lines += edge("run-event", "M454,506 L454,558 L215,558 L215,626", "green")
    lines += label(336, 550, "1 : N", owner="run-event", padded=False)
    lines += edge("idempotency-run", "M322,440 L380,440", "green")
    lines += label(351, 431, "命中 / 新建", owner="idempotency-run", padded=False)
    lines += edge("step-artifact", "M903,506 L903,572 L866,572 L866,626", "gray", dashed=True)
    lines += label(914, 572, "未来", owner="step-artifact", padded=False)

    lines += legend(
        [
            ("blue", "消息正文", False),
            ("purple", "执行结构", False),
            ("green", "事件与幂等", False),
            ("gray", "未来扩展", True),
        ],
        64,
        902,
    )
    lines.append("</svg>")
    return lines


def write_diagram(name: str, lines: list[str]) -> None:
    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)
    (OUTPUT_DIR / f"{name}.svg").write_text("\n".join(lines) + "\n", encoding="utf-8")


def main() -> None:
    diagrams = {
        "agent-system-architecture": architecture_diagram(),
        "agent-layered-architecture": layered_architecture_diagram(),
        "agent-run-sequence": sequence_diagram(),
        "agent-run-state-machine": state_diagram(),
        "agent-data-model": data_model_diagram(),
    }
    for name, lines in diagrams.items():
        write_diagram(name, lines)
        print(f"generated {name}.svg")


if __name__ == "__main__":
    main()
