export default {
  userHome: {
    title: 'Dashboard',
    greeting: {
      morning: 'Good morning, {name}',
      noon: 'Good afternoon, {name}',
      afternoon: 'Good afternoon, {name}',
      evening: 'Good evening, {name}'
    },
    todaySummary: '{requests} requests today, {cost} spent',
    todayIdle: 'No requests yet today',
    actions: {
      createKey: 'Create key',
      createKeyHint: 'Pick a group',
      copyEndpoint: 'Copy endpoint',
      endpointCopied: 'Endpoint copied',
      recharge: 'Top up',
      rechargeHint: 'Open wallet'
    },
    onboarding: {
      title: 'Get started in three steps',
      step1: 'Create an API key',
      step1Hint: 'Pick a group and generate your key',
      step2: 'Copy the endpoint',
      step2Hint: 'Paste it into Claude Code, Codex or Cursor',
      step3: 'Top up',
      step3Hint: 'Add balance before it runs out'
    },
    wallet: {
      label: 'Wallet',
      balance: 'Balance',
      frozen: '{amount} frozen',
      todaySpend: 'Spent today',
      runway: 'Lasts about',
      runwayDays: '~{days} days',
      runwayOverYear: 'Over 1 year',
      runwayUnknown: '—',
      runwayBasis: 'Based on {amount}/day over the last 7 days',
      runwayNoUsage: 'No spending in the last 7 days',
      open: 'Open wallet'
    },
    usage: {
      title: 'Usage',
      periods: {
        today: 'Today',
        week: '7 days',
        month: '30 days'
      },
      requests: 'Requests',
      requestsUnit: 'req',
      tokens: 'Tokens',
      cost: 'Spent',
      speed: 'Avg speed',
      speedUnit: 'tok/s',
      speedHint: 'Median generation speed of streaming requests'
    },
    trend: {
      legendTokens: 'Tokens',
      legendCost: 'Spent',
      empty: 'No usage in this period'
    },
    groups: {
      title: 'By group',
      group: 'Group',
      requests: 'Requests',
      tokens: 'Tokens',
      cost: 'Spent',
      hitRate: 'Cache hit rate',
      hitRateHint: 'Cached tokens are billed at a lower price. The rate depends on your tools and how you chat.',
      tooFew: 'Too few requests to measure',
      deletedGroup: 'Deleted group',
      empty: 'No group usage in this period'
    },
    recent: {
      title: 'Recent requests',
      viewAll: 'View all',
      time: 'Time',
      model: 'Model',
      group: 'Group',
      tokens: 'Tokens in / out',
      cost: 'Spent',
      empty: 'No requests in this period'
    },
    loadFailed: 'Failed to load',
    retry: 'Retry'
  }
}
