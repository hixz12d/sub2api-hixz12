export default {
  userHome: {
    title: '仪表盘',
    greeting: {
      morning: '早上好，{name}',
      noon: '中午好，{name}',
      afternoon: '下午好，{name}',
      evening: '晚上好，{name}'
    },
    todaySummary: '今天已调用 {requests} 次，花费 {cost}',
    todayIdle: '今天还没有调用',
    actions: {
      createKey: '创建 Key',
      createKeyHint: '选分组生成',
      copyEndpoint: '复制接口地址',
      endpointCopied: '接口地址已复制',
      recharge: '充值',
      rechargeHint: '进入钱包充值'
    },
    onboarding: {
      title: '三步开始使用',
      step1: '创建 API Key',
      step1Hint: '选一个分组，生成你的专属密钥',
      step2: '复制接口地址',
      step2Hint: '填进 Claude Code、Codex 或 Cursor',
      step3: '充值',
      step3Hint: '余额用完前记得充值'
    },
    wallet: {
      label: '钱包',
      balance: '余额',
      frozen: '冻结 {amount}',
      todaySpend: '今日花费',
      runway: '预计还能用',
      runwayDays: '约 {days} 天',
      runwayOverYear: '超过 1 年',
      runwayUnknown: '—',
      runwayBasis: '按近 7 天日均 {amount} 估算',
      runwayNoUsage: '近 7 天没有花费',
      open: '进入钱包'
    },
    usage: {
      title: '用量',
      periods: {
        today: '今天',
        week: '7 天',
        month: '30 天'
      },
      requests: '请求数',
      requestsUnit: '次',
      tokens: 'Token',
      cost: '花费',
      speed: '平均速度',
      speedUnit: 'tok/s',
      speedHint: '流式请求的生成速度中位数'
    },
    trend: {
      legendTokens: 'Token',
      legendCost: '花费',
      empty: '这段时间还没有用量'
    },
    groups: {
      title: '按分组',
      group: '分组',
      requests: '请求',
      tokens: 'Token',
      cost: '花费',
      hitRate: '缓存命中率',
      hitRateHint: '命中缓存的部分按更低价格计费。命中率与使用的工具和对话方式有关。',
      tooFew: '请求太少，暂不统计',
      deletedGroup: '已删除的分组',
      empty: '这段时间还没有分组用量'
    },
    recent: {
      title: '最近使用',
      viewAll: '查看全部',
      time: '时间',
      model: '模型',
      group: '分组',
      tokens: 'Token 输入 / 输出',
      cost: '花费',
      empty: '这段时间还没有使用记录'
    },
    loadFailed: '加载失败',
    retry: '重试'
  }
}
