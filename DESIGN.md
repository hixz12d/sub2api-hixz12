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
  gray-50: "#f7f8f6"
  gray-100: "#f1f2f0"
  gray-200: "#e6e8e4"
  gray-300: "#d2d5d0"
  gray-400: "#a3a8a2"
  gray-500: "#8d938d"
  gray-600: "#6a6f6a"
  gray-700: "#464a46"
  gray-800: "#1d1f1d"
  gray-900: "#141514"
  gray-950: "#000000"
  dark-50: "#f7f8f6"
  dark-100: "#e6e8e4"
  dark-200: "#d2d5d0"
  dark-300: "#b9beb9"
  dark-400: "#8d938d"
  dark-500: "#6a6f6a"
  dark-600: "#2a2d2a"
  dark-700: "#1d1f1d"
  dark-800: "#141514"
  dark-900: "#0b0c0b"
  dark-950: "#000000"
  light-bg: "#ffffff"
  light-tile: "#f1f2f0"
  light-tile-2: "#e6e8e4"
  light-ink: "#0a0b0a"
  light-ink-2: "#464a46"
  light-ink-3: "#6a6f6a"
  light-rule: "#dfe1dd"
  light-money: "#00703c"
  light-money-hover: "#005c31"
  light-money-ink: "#ffffff"
  light-money-dim: "#bfe6cf"
  light-money-text: "#00703c"
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
  dark-money-ink: "#ffffff"
  dark-money-dim: "#b8f0cf"
  dark-money-text: "#00a14c"
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
    fontSize: "14px"
    fontWeight: 400
    lineHeight: 1.5
rounded:
  none: "0px"
  hairline: "2px"
spacing:
  tile-gap: "8px"
  row-gap: "4px"
  tile-pad: "18px 20px"
  section: "48px"
  section-mobile: "40px"
  page-x: "32px"
  page-x-tablet: "24px"
  page-x-mobile: "16px"
  content-max: "1320px"
  sidebar: "256px"
  sidebar-collapsed: "72px"
components:
  button-primary:
    backgroundColor: "{colors.light-money}"
    textColor: "{colors.light-money-ink}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "10px 16px"
  button-primary-hover:
    backgroundColor: "{colors.light-money-hover}"
  button-solid:
    backgroundColor: "{colors.light-ink}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "10px 16px"
  button-solid-hover:
    backgroundColor: "{colors.gray-800}"
  button-tile:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "10px 16px"
  button-tile-hover:
    backgroundColor: "{colors.light-tile-2}"
  button-ghost:
    textColor: "{colors.light-ink-2}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "10px 16px"
  button-ghost-hover:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink}"
  button-danger:
    backgroundColor: "{colors.light-bad}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "10px 16px"
  tile:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.none}"
    padding: "{spacing.tile-pad}"
  tile-money:
    backgroundColor: "{colors.light-money}"
    textColor: "{colors.light-money-ink}"
    rounded: "{rounded.none}"
    padding: "{spacing.tile-pad}"
  input:
    backgroundColor: "{colors.light-bg}"
    textColor: "{colors.light-ink}"
    rounded: "{rounded.none}"
    height: "40px"
    padding: "8px 12px"
  segment-active:
    backgroundColor: "{colors.light-money}"
    textColor: "{colors.light-money-ink}"
    rounded: "{rounded.none}"
    height: "36px"
    padding: "0 16px"
  segment-idle:
    backgroundColor: "{colors.light-tile}"
    textColor: "{colors.light-ink-2}"
    rounded: "{rounded.none}"
    height: "36px"
    padding: "0 16px"
  toast:
    backgroundColor: "{colors.gray-950}"
    textColor: "{colors.light-bg}"
    rounded: "{rounded.none}"
    padding: "16px"
  badge-tag:
    backgroundColor: "{colors.light-tile-2}"
    textColor: "{colors.light-ink-2}"
    rounded: "{rounded.none}"
    padding: "2px 8px"
---

# Design System: Sub2API

## Overview

**Creative North Star: "钱是那块颜色"（The Money Tile）**

界面取自 Metro 设计语言的平涂磁贴，并按本产品做了改造：颜色本身就是信息。全站只有一种真正饱和的颜色，就是深绿。它只出现在和钱有关、或者“现在就该点这里”的地方：钱包磁贴、充值按钮、选中的页签、当前菜单、登录页的品牌栏。其余区域都是中性色：亮色主题是纯白底配浅灰磁贴，暗色主题是纯黑底配近黑磁贴。用户扫一眼先看到绿色，也就先看到了余额。

字就是界面。大标题用轻字重、超大字号，问候语能放到海报级；说明小字退成浅灰。层级靠字号和字重拉开，不靠边框、阴影和图标方块。界面里没有渐变、斜面、玻璃效果和投影，每个容器都是一块平涂色块，块与块之间留 8px 缝隙。

亮色、暗色两套主题同等重要，默认跟随系统。管理后台沿用上游的布局，换上本系统的颜色、直角和字体。

**两套界面风格。** 前端同时带着“新版”（本文档描述的风格，默认）和“经典”（上游原版：teal 主色、圆角、阴影、渐变）两套风格。`<html data-ui>` 决定用哪套，取值由 `frontend/src/utils/uiStyle.ts` 从各浏览器自己的 localStorage 读出，切换时整页刷新。

- 两套风格共用同一套 Tailwind 类名：颜色、圆角、阴影、字体、渐变都写成 CSS 变量，新版取值在 `:root` / `html.dark`，经典取值在 `html[data-ui="classic"]`。
- 外壳（侧栏、顶栏、登录注册布局）和部分路由在两套风格下各有一份，新版文件放在 `components/layout/modern/` 等目录。
- 本文档只约束新版，经典段落跟随上游。

**Key Characteristics:**
- 一种饱和绿（钱）+ 略带绿调的中性灰阶；零渐变、零投影、零圆角
- 磁贴网格：平涂色块，8px 缝隙，表格行之间 4px 缝隙，一律左对齐
- 轻字重大字号做标题，所有数字等宽（tabular-nums）
- 每个分组有固定识别色，以 14px 小方块标在名称前，图表系列用同一种颜色
- 状态同时用颜色和图标区分
- 浮层分两种：只放文字的用反色实心块，可交互的用页面底色加 1px 细线

## Colors

中性灰阶略带一点绿，和主色同一个色温；饱和色只有钱绿和分组识别色。所有颜色都写成 CSS 变量（`--c-*`，存 RGB 三元组），Tailwind 用 `rgb(var(--c-x) / <alpha>)` 引用，所以透明度修饰符（如 `bg-money/15`）照常可用。

### Primary
- **钱绿（Money Green）**：语义类 `money`，亮色取 light-money，暗色取 dark-money；白字对比度分别约 6.2:1 和 4.6:1。用在钱包磁贴底色、主按钮、选中的页签和分段控件、进度条填充、开关打开状态、登录页品牌栏、文字选中底色和输入光标（caret）上。悬停时用 `money-hover`：亮色下变深，暗色下变亮。
- **钱绿上的字（money-ink）**：钱绿底上的文字一律用白色。
- **钱绿上的弱字（money-dim）**：钱包磁贴、品牌栏里的标签文字。
- **绿色文字（money-text）**：在中性底上写绿色字时用它，包括当前菜单项、行内代码、文字链接、新手引导序号和 `.text-gradient` 的替代色。暗色下它等于 money-hover，保证在黑底上看得清。
- **焦点环**：3px 实线 `money-hover`，外偏移 2px（全局 `:focus-visible`）。
- **主色色阶 primary-50…950**：给 Tailwind 的 `primary-*` 用，亮暗两套共用同一组值，由模板里的 `dark:` 前缀取不同档位。约定如下：
  - 亮色：实心填充 600，悬停 700，浅底 50/100，文字 700。
  - 暗色：实心填充 500，悬停 400，浅底用 500 加 15% 透明度，文字 300/400。

  新页面优先用语义类 `money*`，不直接写 `primary-*`。

### Secondary（分组识别色）
- **series-1…5**：五种平涂色，用在分组名前 14px 的小方块、图表系列和图例上。亮色组压暗，保证在白底上看得清；暗色组提亮。它们只用来区分分组，不表示好坏。
- 分配规则：分组按花费从高到低排序，按位置取模 5 循环取色；不在列表里的分组按 `abs(id) % 5` 取色。同一页面里，分组表和使用记录用同一套映射。

### Neutral
- **页面底（bg，类名 `surface`）**：亮色纯白，暗色纯黑。
- **磁贴（tile / tile-2，类名 `surface-tile` / `surface-tile-2`）**：用作容器、表格单元格、次级按钮、页签、骨架屏的底色；tile-2 用于悬停和磁贴内再分层。
- **墨色三级（ink / ink-2 / ink-3）**：分别用于正文、次要文字、说明和表头。三级在各自底色上的对比度都 ≥4.5:1。
- **分隔线（rule）**：用于图表网格线、管理后台表格行线、弹窗头尾分隔线和浮层外框。新版把全局默认边框色设成 rule，所以写 `border` 时不用另指定颜色。
- **gray-50…950**：亮色界面用的中性色阶，和 accent-* 同值。常用档位：50/100/200 做底，600/700 做文字；gray-950 是纯黑，用于反色块。
- **dark-50…950**：暗色界面用的中性色阶，**和 gray-* 不同**，按“暗色下的用途”排列：950 是页面底，800 是磁贴，700 是悬停，600 是暗色输入框边框和开关关闭底色，300/400 是次要文字。

### 状态色
- **ok / warn / bad**：分别表示成功、提醒、失败，亮暗各一组。必须配图标一起出现（对勾、感叹号、叉）。文字链接和普通按钮不得借用状态色。
- **缓存命中率**：≥70% 用 ok，40–70% 用 ink-2，<40% 用 warn；请求太少时显示“—”。

### 外部品牌色（例外）
支付按钮保留各支付渠道的品牌色：Stripe `#635bff`、Airwallex `#14171A`（暗色 `#7AF0C4`）、支付宝 `#00AEEF`、微信支付 `#2BB741`。只用在对应渠道的支付按钮上，每个按钮只出现一次，不扩散到其他元素。

### Named Rules
**The One Saturated Color Rule.** 一屏之内，大面积的饱和色只能是钱绿。分组识别色只能出现在 14px 以内的小方块、细条和图表线上。支付品牌色只出现在各自的支付按钮上。

**The Money Tile Rule.** 余额永远放在一块钱绿实心磁贴上，用白字。其他数据磁贴一律用中性色。

**The Semantic First Rule.** 新页面用语义类（`surface*`、`ink*`、`rule`、`money*`、`ok/warn/bad`、`series-*`、`chart-*`），它们会自动跟随主题，也自动兼容经典风格。只有在沿用上游组件时才写 `gray-*` / `dark-*` / `primary-*` 配 `dark:` 前缀。

## Typography

**Display / Body Font:** Anthropic Sans（自托管 `frontend/public/fonts/anthropic-sans.ttf`，只有 400 一个字重），中文回退 PingFang SC / Hiragino Sans GB / Microsoft YaHei
**Mono Font:** Anthropic Mono（自托管 `frontend/public/fonts/anthropic-mono.ttf`，可变字重 300–800）

**Character:** 字体文件只覆盖拉丁字母、数字和符号，中文由系统字体接手，所以数字、金额、模型名、英文按钮带着这套字体的气质，中文保持系统字体的清晰度。Sans 只有 400 一个字重：写 `font-light` 时，中文会用系统字体的细体，拉丁字母和数字仍按 400 显示，加粗则由浏览器合成。这是当前的真实效果，大数字的“轻”主要靠字号和负字距撑起来。

### Hierarchy
- **Display**（300，54px，行高 1.08，字距 -0.035em）：仪表盘问候语、钱包页标题、登录页品牌栏标题。手机降到 40px；登录注册表单的标题固定 40px。
- **Headline**（300，30px，行高 1，字距 -0.02em）：页面内的分区标题（如“用量”“按分组”“最近使用”），以及全局 `.page-title`。手机降到 26px。
- **Metric**（300，40px，行高 1，字距 -0.03em，等宽数字）：数据磁贴里的大数字，单位用 15px ink-3。手机降到 30px。钱包磁贴的数字另有规格，见 Components 的 Wallet Tile。
- **Title**（600，17px）：快捷操作磁贴的标题。弹窗标题用 18px 600。
- **Body**（400，15px，行高 1.5）：用户端正文、侧栏菜单、问候语副标题（手机 15px，桌面 17px，300）。管理后台和全局组件（按钮、输入框、表格）用 14px。
- **Label**（400，13px）：磁贴标签、表头、侧栏分组名、说明文字，颜色用 ink-3 或 ink-2。中文不做大写变换，不加字距。管理后台的全局表头用 12px。
- **Code**（Mono，14px）：模型名、API Key、接口地址、请求 ID。只用于代码和标识符，不当装饰。

### Named Rules
**The Tabular Rule.** 新版的 `body` 全局开启 `font-variant-numeric: tabular-nums`，所有数字（金额、Token、次数、百分比、时间）天然等宽，表格里的数字列右对齐。金额统一写 ¥，大数缩写为 1.2K / 3.4M，整数去掉多余的“.0”。

**The Light Display Rule.** 大字用轻字重（300），重点靠字号拉开；粗体只留给 17px 及以下的标题和强调。

**The Fit-the-Tile Rule.** 磁贴里的大数字随位数逐级缩小，保证不换行、不溢出，不用省略号截断金额。

## Layout

- **磁贴网格**（`.tile-grid`）：容器之间留 8px 缝隙（`gap-tile-gap`），磁贴内边距 18px 20px，分区之间留 48px（手机 40px）。一切左对齐。
- **断点**：用 Tailwind 默认断点：sm 640px、md 768px、lg 1024px、xl 1280px、2xl 1536px。桌面布局从 lg 开始。
- **外壳**：
  - 侧栏宽 256px，折叠后 72px，无边框，和页面同底色。
  - 顶栏高 76px（<lg 时 64px）。
  - 内容区内边距：<md 16px，md 起 24px，lg 起 32px。
  - 用户端页面最大宽度 1320px，居中显示。
  - <lg 时侧栏变成抽屉，从左侧滑出，遮罩为 50% 黑。
- **仪表盘首行**：
  - lg 起四列 `2fr 1fr 1fr 1fr`，钱包磁贴占第一列，高至少 172px；三个快捷操作磁贴依次是充值（钱绿）、创建 Key（反色实心）、复制接口地址（中性灰）。
  - <lg 时两列：钱包全宽排在最上面（高至少 150px），快捷操作磁贴高至少 104px。
- **数据磁贴**：lg 起 4 列，以下 2 列。
- **用户端表格**：`border-separate`，行间 4px 缝隙，每个单元格是一块 tile 底，内边距 14px 18px，整行悬停变成 tile-2。
- **管理后台表格**（全局 `.table`）：透明底，行与行之间用 rule 细线分隔，单元格内边距 12px 16px，悬停变 tile-2。手机端改成卡片列表。
- **登录注册**：lg 起分两栏，左边是钱绿品牌栏（宽 44%，最大 640px，内边距 48px，xl 起 64px），右边是表单列（最大宽 420px）；<lg 时只显示表单列。

## Elevation & Depth

整体完全平面，不用投影，深度只靠底色明度分层：页面底 → tile → tile-2。

浮层分两类：
- **只放文字、不可交互的浮层**用反色实心块，亮色下黑底白字，暗色下白底黑字。包括 Toast（规范）、帮助提示、图表悬停提示、顶栏余额的悬停卡片。
- **可交互的浮层**用页面底色加一圈 1px rule 细线。包括下拉菜单、模态框、对话框；对话框底部的按钮栏用 tile 底。

遮罩：模态框 45% 黑，移动端抽屉 50% 黑，都不模糊背景。

### Shadow Vocabulary
- **细线外框**（`box-shadow: 0 0 0 1px rgb(var(--c-rule))`）：新版把 Tailwind 的 `shadow-lg/xl/2xl` 改成了这条 1px 外框。上游组件写的大阴影会自动变成细线，`shadow-sm/md/DEFAULT`、`shadow-card`、`shadow-glass`、`shadow-glow` 则一律为空。
- **粘性表头分隔**（`box-shadow: 0 1px 0 rgb(var(--c-rule))`）：表格表头固定时，用它和下方滚动内容分开。

### Named Rules
**The Flat Rule.** 不用 box-shadow 表达层级，也不用玻璃和模糊。需要“浮起”时，改底色明度、换成反色块，或者加一圈 1px rule。

## Shapes

全站直角（0px）。新版把 Tailwind 的 `rounded`、`rounded-md` 到 `rounded-4xl` 都映射成 0，只有 `rounded-sm` 保留 2px，只用在复选框这类极小的元素上。`rounded-full` 仍然是正圆，只允许用在加载转圈和状态小圆点上；开关、头像、进度条、标签一律用直角。

头像是方块，菜单当前项前的标记、分组识别标记都是实心方块。

图标用 `components/icons/Icon.vue` 自带的 Heroicons outline 路径集合（项目没有装图标库）：24×24 画布，线宽 1.5，圆头圆角。尺寸档：xs 12px、sm 16px、md 20px（默认）、lg 24px、xl 32px。侧栏图标 20px，子菜单 16px，快捷操作磁贴 24px。

## Components

### Buttons
- **Shape:** 直角，默认高 40px（左右 16px，14px 字，字重 600），登录注册的提交按钮高 48px、15px 字。尺寸档：`btn-sm`（12px 字）、`btn-md`、`btn-lg`（16px 字）、`btn-icon`（四周 10px）。
- **Primary（钱绿，`btn-primary`）：** 用于充值、确认支付、保存这类主操作，每个区域最多一个。
- **Solid（反色实心，`btn-solid`）：** 亮色黑底白字，暗色白底黑字。用于第二重要的操作，如“创建 Key”。
- **Secondary（中性磁贴，`btn-secondary`）：** tile 底、ink 字，悬停变 tile-2。其余操作都用它。
- **Ghost（`btn-ghost`）：** 透明底、ink-2 字，悬停时出现 tile 底、文字变 ink。用于工具栏和行内的次要操作。
- **Danger（`btn-danger`）：** 亮暗两套都是 `#b4002a` 底白字，悬停 `#8f0021`。只用于删除、停用这类不可逆操作。
- **钱绿磁贴上的按钮：** 钱包页余额卡上的充值按钮是白底黑字，高 44px。
- **Hover / Focus / Active：** 悬停只换底色，过渡 150ms；键盘焦点见 Colors 的焦点环；按下时缩到 0.985；禁用时透明度 50%。复制成功时按钮或磁贴短暂变成 ok 底白字，文字变成“已复制”。

### Tiles / Containers
- **Corner Style:** 0px。
- **Background:** 中性磁贴用 `.tile`（tile 底）；钱绿磁贴用 `.tile-money`；上游的 `.card` / `.glass-card` 在新版下也是平涂 tile 底，悬停版（`.card-hover`）变 tile-2。
- **Shadow Strategy:** 无（见 Elevation & Depth）。
- **Border:** 无。磁贴不嵌套磁贴，需要再分块时用 tile-2 或 rule 细线（`.card-header` / `.card-footer` 就是 rule 细线）。
- **Internal Padding:** 18px 20px；大块容器可到 22px 24px（`.card-body` 为 24px）。

### Inputs / Fields
- **Style:** 直角，高约 40px（上下 8px、左右 12px，14px 字），页面底色，2px 边框（亮色 gray-300，暗色 dark-600），文字 ink，占位文字 ink-3。
- **Label / Hint：** 标签 14px 500 ink-2，下方留 6px；提示 12px ink-3。
- **Focus:** 边框变成钱绿，不加光晕。
- **Error / Disabled:** 错误时边框和说明文字用 bad，并配叉号图标；禁用时底色换成 tile，文字变 ink-3。
- **开关（`.switch`）：** 方形 44×24px，关闭时亮色 gray-300、暗色 dark-600 底，打开时钱绿底；滑块是 20px 白色方块。

### Segmented Control / Tabs
- 每个选项是一块独立的中性小磁贴，间隔 4px，高 36px，14px 600。选中项被钱绿“灌满”，白字；未选中项是 tile 底、ink-2 字，悬停变 tile-2。
- 全局 `.tab` / `.tab-active` 同样是这个样式。钱包的四个页签（充值、兑换码、订单、邀请返利）和仪表盘的时间范围切换都用它。

### Badges / Status
- **状态：** 用户端写成“图标 + 文字”，颜色用 ok / warn / bad，不加底色块。
- **标签（`.badge-gray`）：** tile-2 底、ink-2 字，12px 500，直角，左右 8px。
- **后台状态徽章（`.badge-primary/success/warning/danger/purple`）：** 低饱和浅底，亮色用 100 档底、700/800 档字，暗色用 500 档 15% 透明度底、300 档字。只用在管理后台的密集表格里。
- **分组标识：** 14px 实心小方块（series 色）+ 分组名。只显示分组名称，不显示数字 ID。

### Tables
- 表头：13px（后台 12px），ink-3，无底色。
- 行：用户端见 Layout 的表格缝隙；数字列右对齐，数字等宽。
- 比例条（如缓存命中率）：宽 96px、高 8px 的平涂细条，底色 tile-2，填充为状态色；左边是右对齐的百分比数字。不用进度环。
- 通用进度条（`.progress`）：8px 高，tile-2 底，钱绿填充，直角。

### Navigation
- **侧栏：**
  - 头部高 64px。
  - 组名 13px ink-3，下方留 6px；折叠后组名变成一条 1px rule 细线。
  - 菜单项 15px ink-2，上下 8px、左边 17px，悬停时只把文字变成 ink。
  - 当前项是加粗的 money-text 文字，前面一个 8px 实心方块（`::before`，距左 3px，颜色同文字）。
  - 子菜单向右缩进 16px，左边一条 rule 细线。
- **顶栏：** 高 76px（<lg 时 64px）。左边是页面名（15px ink-3）。右边依次是余额块、主题切换、语言切换、头像方块：
  - 余额块：钱绿实心，高 40px，点进钱包；悬停弹出反色卡片，宽 224px。
  - 头像方块：40px，反色。
- **手机端：** 侧栏变成从左滑出的抽屉。

### Dialogs / Toast
- **模态框 / 对话框：** 页面底色 + 1px rule 外框，头部和底部用 rule 细线分开，内边距 16–24px，标题 18px 600。入场时从下方 12px 处滑上并淡入，250ms，`cubic-bezier(.2,.9,.2,1)`；退场 200ms。
- **Toast：** 规范是全局 `.toast`：反色实心块，右上角，最小宽 320px，内边距 16px，从右侧滑入；不加左侧彩色竖条，用图标区分类型。实际弹出的 `Toast.vue` 还没改，见下方“存量偏差”。

### Charts
- **首页趋势图**（`HomeTrendChart.vue`）：
  - Token 用柱：用 chart-bar 色（亮色近黑，暗色近白），直角，每组柱占格宽 70%。
  - 花费用折线：3px chart-line 色，不画圆点（悬停时显示 4px 圆点），直线连接，不平滑。
  - 网格线用 rule，坐标文字 11px ink-3，不显示图例。
  - 悬停提示是反色实心块，直角，内边距 10px，不显示颜色方块。
  - 颜色从 CSS 变量实时读取，切换主题时重绘。
- **多系列（按分组）：** 按 series-1…5 依次取色。
- **入场：** 500ms `easeOutQuart`；开启“减少动画”时关闭。

### Wallet Tile（签名组件）
- 整块是一块钱绿实心磁贴，可点，点击进入钱包，悬停时底色变成 money-hover。三个数同时常驻显示，不轮播。
- 网格：两列 `1.3fr 1fr` × 两行。左格占满两行：
  - 标签“余额”（14px 500 money-dim）。
  - 下方是余额大字（字重 400，行高 1，字距 -0.035em）。字号随位数缩小：≤7 位时 40 / 44 / 56px（手机 / sm / 2xl），≤9 位时 34 / 38 / 46px，更长时 28 / 32 / 38px。
- 右上格：标签“今日花费”，金额 24px，sm 起 32px，字重 500。余额和今日花费是这块磁贴上最显眼的两个数。
- 右下格：标签“预计还能用”，“约 19 天”17px，sm 起 20px。下一行小字说明估算依据，如“按近 7 天日均 ¥6.75”，手机端隐藏这行小字。最近没有用量时，整格不显示天数。
- 格与格之间用 30% 透明度的白色 1px 细线分开；加载中显示 20% 透明度的白色脉冲占位块。
- 钱包页的余额卡是简化版：钱绿底，最小高 132px，余额 44px，sm 起 56px，右侧放一个白底充值按钮。

### Quick Action Tile
- 整块可点的大按钮磁贴：最小高 104px，内边距 18px 20px，标题 17px 600，提示 13px（透明度 80%），图标 24px。三种配色和 Buttons 的 Primary / Solid / Secondary 一一对应；按下时缩到 0.985。
- 新用户引导时，磁贴改成显示步骤序号（40px 300 money-text）。

### Motion
- **入场：** 首屏元素加 `.tile-glide`，从右侧 28px 处滑入并淡入，0.5s，`cubic-bezier(.2,.9,.2,1)`。用 `style="--n:序号"` 依次错开 55ms，只在页面首次加载时播放。登录注册布局自带一份同样的 `.auth-glide`。
- **状态变化：** 底色和文字颜色过渡 150ms；切换时间范围时数字直接替换，图表重新播放入场动画。
- **按下：** 只缩放到 0.985，不做倾斜。
- **减少动画：** `prefers-reduced-motion: reduce` 时，新版把全部动画和过渡缩短到 0.01ms（加载转圈除外），模态框不再位移，图表关闭动画。

### 存量偏差（新代码不得照抄）
以下是上游遗留或尚未改完的写法，在新版下仍然会显出来：
- `rounded-full`：`Toggle.vue` 的胶囊开关、骨架屏圆形、进度条。新开关用 `.switch`。
- `Toast.vue` 没用全局 `.toast`：白底（暗色 dark-800）、左侧 4px 状态色竖条、1px 细线外框，和反色块规范不一致。
- 渐变和模糊：
  - 顶栏用了 `backdrop-blur-sm`。
  - 公告铃铛和公告弹窗用了渐变、`blur-3xl` 光斑和彩色阴影。
  - 首页落地页的终端窗口用了倾斜、渐变、14px 圆角和重阴影。
  - `ModelIcon` 写死了 4px 圆角和靛蓝渐变。
- `components/charts/*` 的旧图表（管理后台仪表盘和用量页、用户用量页在用）：写死了 Tailwind 调色板（蓝、绿、黄、红、紫），折线平滑并填充面积，不跟随主题变量。
- `DataTable` 的滚动条写死了 6px 圆角，滚动阴影用了渐变。

## Do's and Don'ts

### Do:
- **Do** 让余额和今日花费永远同时出现在钱绿实心磁贴上，并在手机端放在最上方。
- **Do** 新页面用语义类（`surface*`、`ink*`、`money*`、`rule`、`ok/warn/bad`、`series-*`），用现成的 `.tile`、`.tile-money`、`.tile-grid`、`.btn-*`、`.tab`、`.input`、`.switch`，并且新页面写成新文件。
- **Do** 所有数字等宽，金额统一写 ¥，大数缩写为 1.2K / 3.4M；磁贴里的大数字随位数缩小，不截断。
- **Do** 状态同时用颜色和图标表达；“请求太少”时显示“—”，并给出悬停说明。
- **Do** 亮色和暗色都检查一遍：正文对比度 ≥4.5:1，暗色下的绿色文字用 money-text。
- **Do** 浏览器自带的部分也按主题处理：文字选中底色用钱绿，caret 用钱绿，滚动条细且平时透明，焦点环是 3px money-hover。
- **Do** 新增的样式规则区分新版和经典：用 `:where(html:not([data-ui='classic']))` / `:where(html[data-ui='classic'])` 做前缀，不用 CSS 嵌套。

### Don't:
- **Don't** 用渐变、玻璃模糊、投影、发光，也不要沿用经典风格的 mesh 背景。
- **Don't** 给容器加圆角或描边，不要把磁贴嵌进磁贴，也不要在开关、头像、标签上用 `rounded-full`。
- **Don't** 在标题上方加小号“眉题”标签，也不要用渐变文字。
- **Don't** 把分组识别色、状态色或支付品牌色铺成大面积底色；大面积的饱和色只能是钱绿。
- **Don't** 用“彩色图标方块 + 大数字”的模板化统计卡，也不要用进度环。
- **Don't** 给 Toast 加左侧彩色竖条，也不要在按下时做倾斜或弹跳。
- **Don't** 用 emoji 或 Unicode 符号代替图标，也不要在用户可见处显示分组的数字 ID。
