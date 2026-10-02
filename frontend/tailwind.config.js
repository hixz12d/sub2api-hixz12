/** @type {import('tailwindcss').Config} */

// 颜色来自 style.css 里的 CSS 变量（亮/暗两套），规范见仓库根目录 DESIGN.md
const scale = (name) =>
  Object.fromEntries(
    [50, 100, 200, 300, 400, 500, 600, 700, 800, 900, 950].map((step) => [
      step,
      `rgb(var(--c-${name}-${step}) / <alpha-value>)`
    ])
  )
const token = (name) => `rgb(var(--c-${name}) / <alpha-value>)`

export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    // 全站直角；full 保留给圆点、开关、加载圈
    borderRadius: {
      none: '0px',
      sm: '2px',
      DEFAULT: '0px',
      md: '0px',
      lg: '0px',
      xl: '0px',
      '2xl': '0px',
      '3xl': '0px',
      '4xl': '0px',
      full: '9999px'
    },
    // 平面设计：小阴影取消，浮层改为 1px 细线外框
    boxShadow: {
      none: 'none',
      sm: 'none',
      DEFAULT: 'none',
      md: 'none',
      lg: '0 0 0 1px rgb(var(--c-rule))',
      xl: '0 0 0 1px rgb(var(--c-rule))',
      '2xl': '0 0 0 1px rgb(var(--c-rule))',
      inner: 'none'
    },
    extend: {
      colors: {
        // 主色：钱绿
        primary: scale('primary'),
        // 中性色：亮色界面用 gray，暗色界面用 dark
        gray: scale('gray'),
        dark: scale('dark'),
        accent: scale('gray'),
        // 语义色：随主题自动切换，新页面优先用这些
        surface: { DEFAULT: token('bg'), tile: token('tile'), 'tile-2': token('tile-2') },
        ink: { DEFAULT: token('ink'), 2: token('ink-2'), 3: token('ink-3') },
        rule: token('rule'),
        money: {
          DEFAULT: token('money'),
          hover: token('money-hover'),
          ink: token('money-ink'),
          dim: token('money-dim'),
          text: token('money-text')
        },
        ok: token('ok'),
        warn: token('warn'),
        bad: token('bad'),
        series: {
          1: token('series-1'),
          2: token('series-2'),
          3: token('series-3'),
          4: token('series-4'),
          5: token('series-5')
        },
        chart: { bar: token('chart-bar'), line: token('chart-line') }
      },
      fontFamily: {
        sans: [
          'Anthropic Sans',
          'Segoe UI',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'Helvetica Neue',
          'Arial',
          'PingFang SC',
          'Hiragino Sans GB',
          'Microsoft YaHei',
          'sans-serif'
        ],
        mono: ['Anthropic Mono', 'ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
      },
      boxShadow: {
        glass: 'none',
        'glass-sm': 'none',
        glow: 'none',
        'glow-lg': 'none',
        card: 'none',
        'card-hover': 'none',
        'inner-glow': 'none'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(rgb(var(--c-money)), rgb(var(--c-money)))',
        'gradient-dark': 'linear-gradient(rgb(var(--c-tile)), rgb(var(--c-tile)))',
        'gradient-glass': 'none',
        'mesh-gradient': 'none'
      },
      spacing: {
        'tile-gap': '8px'
      },
      transitionTimingFunction: {
        glide: 'cubic-bezier(.2,.9,.2,1)'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'none',
        glide: 'glide 0.5s cubic-bezier(.2,.9,.2,1) both'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glide: {
          '0%': { opacity: '0', transform: 'translateX(28px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      }
    }
  },
  plugins: []
}
