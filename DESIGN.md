---
name: Sub2API
description: AI API 网关的用户端与管理后台：大色块承载钱，字就是界面
colors:
  primary-50: "#e8f6ee"
  primary-100: "#c9ebd7"
  primary-200: "#97d8b2"
  primary-300: "#5ec289"
  primary-400: "#2ea866"
  primary-500: "#00873f"
  primary-600: "#00703c"
  primary-700: "#005c31"
  primary-800: "#004a28"
  primary-900: "#003b20"
  primary-950: "#002213"
  neutral-50: "#f7f8f6"
  neutral-100: "#f1f2f0"
  neutral-200: "#e6e8e4"
  neutral-300: "#d2d5d0"
  neutral-400: "#a3a8a2"
  neutral-500: "#8d938d"
  neutral-600: "#6a6f6a"
  neutral-700: "#464a46"
  neutral-800: "#1d1f1d"
  neutral-900: "#141514"
  neutral-950: "#000000"
  light-bg: "#ffffff"
  light-tile: "#f1f2f0"
  light-tile-2: "#e6e8e4"
  light-ink: "#0a0b0a"
  light-ink-2: "#464a46"
  light-ink-3: "#6a6f6a"
  light-rule: "#dfe1dd"
  light-money: "#00703c"
  light-money-hover: "#005c31"
  light-ok: "#00703c"
  light-warn: "#a24b00"
  light-bad: "#b4002a"
  dark-bg: "#000000"
  dark-tile: "#141514"
  dark-tile-2: "#1d1f1d"
  dark-ink: "#ffffff"
  dark-ink-2: "#b9beb9"
  dark-ink-3: "#8d938d"
  dark-rule: "#262826"
  dark-money: "#00873f"
  dark-money-hover: "#00a14c"
  dark-ok: "#34d17a"
  dark-warn: "#ffa53a"
  dark-bad: "#ff6b85"
  light-series-1: "#00703c"
  light-series-2: "#0050b4"
  light-series-3: "#a24b00"
  light-series-4: "#8a1c9c"
  light-series-5: "#6a6f6a"
  dark-series-1: "#34d17a"
  dark-series-2: "#4d9bff"
  dark-series-3: "#ffa53a"
  dark-series-4: "#e07bf0"
  dark-series-5: "#8d938d"
  light-chart-bar: "#0a0b0a"
  light-chart-line: "#00a85a"
  dark-chart-bar: "#e7eae7"
  dark-chart-line: "#34d17a"
typography:
  display:
    fontFamily: "'Anthropic Sans', 'Segoe UI', system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "54px"
    fontWeight: 300
    lineHeight: 1.08
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "'Anthropic Sans', 'Segoe UI', system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "30px"
    fontWeight: 300
    lineHeight: 1
    letterSpacing: "-0.02em"
  metric:
    fontFamily: "'Anthropic Sans', 'Segoe UI', system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "40px"
    fontWeight: 300
    lineHeight: 1
    letterSpacing: "-0.03em"
    fontFeature: "'tnum'"
  title:
    fontFamily: "'Anthropic Sans', 'Segoe UI', system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "17px"
    fontWeight: 600
    lineHeight: 1.35
  body:
    fontFamily: "'Anthropic Sans', 'Segoe UI', system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.5
  label:
    fontFamily: "'Anthropic Sans', 'Segoe UI', system-ui, 'PingFang SC', 'Microsoft YaHei', sans-serif"
    fontSize: "13px"
    fontWeight: 400
    lineHeight: 1.4
  code:
    fontFamily: "'Anthropic Mono', ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
    fontSize: "13.5px"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  none: "0px"
  hairline: "2px"
spacing:
  tile-gap: "8px"
  tile-pad: "18px 20px"
  section: "48px"
  page-x: "32px"
  page-x-mobile: "16px"
components:
  button-primary:
    backgroundColor: "{colors.light-money}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "0 16px"
  button-primary-hover:
    backgroundColor: "{colors.light-money-hover}"
  button-solid:
    backgroundColor: "{colors.light-ink}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "0 16px"
  button-tile:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "0 16px"
  button-tile-hover:
    backgroundColor: "{colors.light-tile-2}"
  button-danger:
    backgroundColor: "{colors.light-bad}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "0 16px"
  tile:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.none}"
    padding: "{spacing.tile-pad}"
  tile-money:
    backgroundColor: "{colors.light-money}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    padding: "{spacing.tile-pad}"
  input:
    backgroundColor: "{colors.light-bg}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "0 12px"
  segment-active:
    backgroundColor: "{colors.light-money}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    height: "36px"
    padding: "0 16px"
  segment-idle:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink-2}"
    rounded: "{rounded.none}"
    height: "36px"
    padding: "0 16px"
---

# Design System: Sub2API

## Overview

**Creative North Star: "钱是那块颜色"（The Money Tile）**

界面源自 Metro 设计语言的平涂磁贴，按本产品改造：颜色本身就是信息。全站只有一块真正饱和的颜色，深绿，它只出现在和钱有关、或者“现在就该点这里”的地方，比如钱包磁贴、充值按钮、选中的页签和当前菜单。其余区域都是中性的：亮色是纯白底配浅灰磁贴，暗色是纯黑底配近黑磁贴。用户扫一眼，先看到绿色，也就先看到了余额。

字就是界面。大标题用轻字重、超大字号，问候语可以到海报级；说明小字退到浅灰。层级靠字号和字重拉开，不靠边框、阴影和图标方块。没有渐变、斜面、玻璃和投影，容器就是一块平涂色块，之间用 8px 缝隙分开。

两套主题同等重要，默认跟随系统。管理后台沿用自己的布局，只换颜色、直角和字体。

**Key Characteristics:**
- 一块饱和绿（钱）+ 中性灰阶，零渐变、零投影、零圆角
- 磁贴网格：平涂色块，8px 缝隙，左对齐
- 轻字重大字号做标题，数字一律等宽（tabular-nums）
- 分组有固定识别色，用小方块标在名称前，图表系列同色
- 状态同时用颜色和图标区分

## Colors

中性灰阶略带一点绿，和主色同一个色温；饱和色只有绿色和分组识别色。

### Primary
- **钱绿（Money Green）**：亮色用 light-money，暗色用 dark-money（白字对比分别约 6.2:1 和 4.6:1）。用于钱包磁贴底色、主按钮、选中的页签和分段控件、当前菜单项、焦点环、文字选中底色。悬停加深为 light-money-hover，暗色下提亮为 dark-money-hover。暗色下绿色文字（如当前菜单）用 dark-money-hover，保证在黑底上可读。
- **主色色阶 primary-50…950**：给 Tailwind 的 `primary-*` 用，亮暗两套共用同一组值，由代码里的 `dark:` 前缀选不同档位。约定：亮色里实心填充用 600、悬停 700、浅底 50/100、文字 700；暗色里实心填充用 500、悬停 400、浅底用 500 加 15% 透明度、文字 300/400。

### Secondary（分组识别色）
- **series-1…5**：五个平涂色，按分组出现顺序循环分配，用在分组名前 14px 的小方块、图表系列和图例上。亮色组压暗，保证在白底上看得清；暗色组提亮。它们只用来认分组，不表示好坏。

### Neutral
- **页面底（bg）**：亮色纯白，暗色纯黑。
- **磁贴（tile / tile-2）**：容器、表格行、次级按钮、输入区的底色；tile-2 用于悬停和磁贴内再分层。
- **墨色三级（ink / ink-2 / ink-3）**：正文、次要文字、说明和表头。三级在各自底色上的对比都 ≥4.5:1。
- **分隔线（rule）**：只用于图表网格和表格内的细线，不用来给容器描边。
- **中性色阶 neutral-50…950**：给 Tailwind 的 `gray-*` 和 `dark-*` 用，两套主题共用。亮色界面常用 50/100/200 做底、600/700 做文字；暗色界面常用 950 做页底、900 做磁贴、800 做悬停和输入底、400/500 做次要文字。

### 状态色
- **ok / warn / bad**：成功、提醒、失败，亮暗各一组。必须配图标一起出现（对勾、感叹号、叉）；文字链接和按钮不得借用状态色。缓存命中率 ≥70% 用 ok，40–70% 用 ink-2，<40% 用 warn，请求太少显示“—”。

### Named Rules
**The One Saturated Color Rule.** 一屏之内，大面积的饱和色只能是钱绿。分组识别色只能出现在 14px 以内的小方块、细条和图表线上。

**The Money Tile Rule.** 余额永远在一块钱绿实心磁贴上，白字。其他数据磁贴一律中性。

## Typography

**Display / Body Font:** Anthropic Sans（自托管 `frontend/public/fonts/anthropic-sans.ttf`），中文回退 PingFang SC / Microsoft YaHei
**Mono Font:** Anthropic Mono（自托管 `frontend/public/fonts/anthropic-mono.ttf`）

**Character:** 字体只覆盖拉丁字母、数字和符号，中文由系统字体接手。所以数字、金额、模型名、英文按钮会带上这套字体的气质，中文保持系统的清晰度。

### Hierarchy
- **Display**（300，54px，行高 1.08，字距 -0.035em）：仪表盘问候语、登录注册页大标题。手机降到 40px。
- **Headline**（300，30px，行高 1）：页面内分区标题，如“用量”“按分组”“最近使用”。手机 26px。
- **Metric**（300，40px，行高 1，等宽数字）：数据磁贴里的大数字。手机 30px。钱包磁贴的数字另有规格，见 Components 的 Wallet Tile。
- **Title**（600，17px）：磁贴按钮文字、卡片标题、弹窗标题。
- **Body**（400，15px，行高 1.5）：用户端正文。管理后台表格密集，正文可用 14px。
- **Label**（400，13px）：磁贴标签、表头、说明，颜色用 ink-3 或 ink-2。中文不做大写变换，不加字距。
- **Code**（Mono，13.5px）：模型名、API Key、接口地址、请求 ID。只用于代码和标识符，不当装饰。

### Named Rules
**The Tabular Rule.** 所有数字（金额、Token、次数、百分比、时间）都开 `font-variant-numeric: tabular-nums`，表格里的数字列右对齐。金额统一写 ¥，大数缩写为 1.2K / 3.4M，整数位去掉多余的“.0”。

**The Light Display Rule.** 大字用轻字重（300），重点靠字号；粗体只留给 17px 以下的标题和强调。

## Layout

- **磁贴网格**：容器之间 8px 缝隙，磁贴内边距 18px 20px，分区之间 48px。一切左对齐。
- **页面**：侧栏 240px，无边框，和页面同底色；内容区左右边距 32px，最大宽度约 1320px。手机端（≤860px）侧栏变抽屉，内容边距 16px。
- **仪表盘首行**（高 172px）：钱包磁贴占 2 份，三个快捷操作磁贴各占 1 份（充值是钱绿，创建 Key 是反色实心，复制接口地址是中性灰）。≤1180px 时钱包磁贴独占一行；手机端钱包全宽在最上，充值全宽，其余两两一行。
- **数据磁贴**：桌面 4 列，≤1180px 2 列，手机 2 列。
- **表格**：行与行之间留 4px 缝隙，每行是一条磁贴（`border-spacing: 0 4px`）。管理后台的密集表格可以改用细线分隔（rule），行高不变。

## Elevation & Depth

完全平面，没有投影。深度只靠底色明度分层：页面底 → tile → tile-2。浮层（下拉、弹窗、提示）用反色实心块（亮色下黑底白字，暗色下白底黑字），或 tile-2 加一圈 1px rule，不加投影。弹窗遮罩是 45% 黑。

### Named Rules
**The Flat Rule.** 不用 box-shadow 表达层级，也不用玻璃和模糊。需要“浮起”时改底色明度，或换成反色块。

## Shapes

全站直角（0px）。唯一的例外是 2px 细圆角，只能用在极小的元素上（复选框、比例条），也可以不用。头像是方块。图标统一 Lucide 线性图标，线宽 1.5–1.6，20px；在 13px 文字旁缩到 15–16px。

## Components

### Buttons
- **Shape:** 直角，高 40px（手机 44px），左右 16px，字重 600。
- **Primary（钱绿）：** 充值、确认支付、保存这类主操作，每个区域最多一个。
- **Solid（反色实心）：** 亮色黑底白字，暗色白底黑字。用于第二重要的操作，如“创建 Key”。
- **Tile（中性）：** tile 底、ink 字，悬停 tile-2。其余所有操作都用它。
- **Danger：** bad 底白字，只用于删除、停用这类不可逆操作。
- **Hover / Focus / Active：** 悬停只换底色（150ms）；键盘焦点是 3px 钱绿外环、偏移 2px；按下时朝指针方向倾斜 ≤8°、缩到 0.985（开启“减少动画”时取消）。复制成功时按钮短暂变成 ok 底白字，文字变成“已复制”。

### Tiles / Containers
- **Corner Style:** 0px。
- **Background:** tile；钱包磁贴用钱绿；快捷操作磁贴按 Buttons 的三种配色。
- **Shadow Strategy:** 无（见 Elevation & Depth）。
- **Border:** 无。不嵌套磁贴；磁贴里需要再分块时用 tile-2 或细线。
- **Internal Padding:** 18px 20px；大磁贴可到 22px 24px。

### Inputs / Fields
- **Style:** 直角，高 40px，底色为页面底，2px 边框（亮色 neutral-300，暗色 neutral-800），文字 ink，占位 ink-3。
- **Focus:** 边框变钱绿，不加光晕。
- **Error / Disabled:** 错误时边框和说明文字用 bad，并配叉号图标；禁用时底色换成 tile，文字 ink-3。

### Segmented Control / Tabs
- 每个选项是一块独立的中性小磁贴，间隔 4px；选中的那块被钱绿“灌满”，白字。页签同理：选中项是钱绿实心块，其他是中性块。钱包的四个页签（充值、兑换码、订单、邀请返利）用这个样式。

### Badges / Status
- **状态：** 图标 + 文字，颜色用 ok / warn / bad，不加底色块。
- **标签：** tile 底、ink-2 字，13px，直角，左右 8px。
- **分组标识：** 14px 实心小方块（series 色）+ 分组名。只显示分组名称，不显示数字 ID。

### Tables
- 表头：13px，ink-3，无底色。
- 行：tile 底，悬停 tile-2，行间 4px 缝隙；数字列右对齐、等宽。
- 比例条（如缓存命中率）：宽 96px、高 8px 的平涂细条，底色 tile-2，填充为状态色；左边是右对齐的百分比数字。不用进度环。

### Navigation
- **侧栏：** 无框，和页面同底色。组名 13px ink-3（常用 / 钱 / 查看 / 账户），菜单项 16px ink-2，行高约 34px。当前项是钱绿加粗文字，前面一个 8px 实心方块；悬停只把文字变成 ink。
- **顶栏：** 高 76px（手机 60px），无底色和边框。左边是页面名（15px ink-3），右边是余额磁贴（钱绿实心，点进钱包）、主题切换、语言切换、头像方块（反色）。
- **手机端：** 侧栏变成从左滑出的抽屉，遮罩 45% 黑。

### Charts
- Token 用柱（亮色近黑，暗色近白，柱宽占格 70%，直角）；花费用 3px 钱绿折线（chart-line），不画圆点。网格线用 rule，坐标文字 11px ink-3。
- 悬停提示是反色实心块。多系列（按分组）时按 series-1…5 依次取色。
- 入场：柱子从底部长出（每根错开 16ms），折线随后描出。

### Wallet Tile（签名组件）
- 钱绿实心磁贴，整块可点，点击进入钱包，悬停时底色加深。三个数同时常驻，不轮播：
  - 左格占整高：标签“余额”，下方余额大字（60px，字重 400，手机 40px）。
  - 右上格：标签“今日花费”，金额 32px、字重 500（手机 24px）。余额和今日花费是这块磁贴最显眼的两个数。
  - 右下格：标签“预计还能用”，“约 19 天”20px，下一行小字说明估算依据，如“按近 7 天日均 ¥6.75”（手机端隐藏这行小字）。最近没有用量时整格不显示天数。
- 格与格之间用 28% 白色的 1px 细线分开；标签用 money-dim 色（亮色 #bfe6cf，暗色 #b8f0cf）。
- 桌面端首行高 172px；手机端钱包磁贴全宽，高 150px。

### Motion
- **入场：** 首屏元素从右侧 28px 处滑入并淡入，0.5s，`cubic-bezier(.2,.9,.2,1)`，依次错开 55ms，只在页面首次加载时播放一次。
- **状态变化：** 底色和颜色过渡 150ms；切换时间范围时数字直接替换，图表重新长出。
- **减少动画：** `prefers-reduced-motion: reduce` 时关闭全部动画和过渡。

## Do's and Don'ts

### Do:
- **Do** 让余额和今日花费永远同时出现在钱绿实心磁贴上，并在手机端放在最上方。
- **Do** 所有数字开等宽数字，金额统一写 ¥，大数缩写为 1.2K / 3.4M。
- **Do** 状态同时用颜色和图标表达；“请求太少”显示“—”，并给出悬停说明。
- **Do** 亮色和暗色都检查一遍：正文对比 ≥4.5:1，暗色下的绿色文字用 dark-money-hover。
- **Do** 浏览器自带的部分也按主题处理：文字选中底色用钱绿、caret 用钱绿、细滚动条、3px 钱绿焦点环。

### Don't:
- **Don't** 用渐变、玻璃模糊、投影、发光，也不要沿用旧版的 mesh 背景。
- **Don't** 给容器加圆角或描边，也不要把磁贴嵌进磁贴。
- **Don't** 在标题上方加小号“眉题”标签，也不要用渐变文字。
- **Don't** 把分组识别色或状态色铺成大面积底色；大面积饱和色只能是钱绿。
- **Don't** 用彩色图标方块加大数字的模板化统计卡，也不要用进度环。
- **Don't** 用 emoji 或 Unicode 符号代替图标，也不要在用户可见处显示分组的数字 ID。
