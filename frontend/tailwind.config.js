/** @type {import('tailwindcss').Config} */

// 两套风格（经典 / 新版）共用这一份配置：凡是两边取值不同的设计参数（颜色、圆角、阴影、字体、渐变、glow 动画）
// 都写成 CSS 变量，具体取值在 src/style.css 里按 :root（新版）和 html[data-ui="classic"]（经典）各填一套。
// 新版规范见仓库根目录 DESIGN.md；经典版取值与上游 tailwind.config.js 一致。
// 已知限制：阴影写成变量后，shadow-primary-500/25 这类“阴影颜色”类不再给阴影上色，显示为普通阴影。
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
    // 圆角：新版全站直角（sm 2px，其余 0）；经典版为 Tailwind 默认值。none、full 两边固定
    borderRadius: {
      none: '0px',
      sm: 'var(--ui-radius-sm)',
      DEFAULT: 'var(--ui-radius)',
      md: 'var(--ui-radius-md)',
      lg: 'var(--ui-radius-lg)',
      xl: 'var(--ui-radius-xl)',
      '2xl': 'var(--ui-radius-2xl)',
      '3xl': 'var(--ui-radius-3xl)',
      '4xl': 'var(--ui-radius-4xl)',
      full: '9999px'
    },
    // 阴影：新版小阴影取消、浮层为 1px 细线外框；经典版为 Tailwind 默认阴影。
    // lg/xl/2xl 第一层是新版的细线外框（经典版透明度为 0），这样新版配合 shadow-black/10 等颜色类时细线照旧变色
    boxShadow: {
      none: 'none',
      sm: 'var(--ui-shadow-sm)',
      DEFAULT: 'var(--ui-shadow)',
      md: 'var(--ui-shadow-md)',
      lg: '0 0 0 1px rgb(var(--c-rule) / var(--ui-shadow-ring-alpha)), var(--ui-shadow-lg)',
      xl: '0 0 0 1px rgb(var(--c-rule) / var(--ui-shadow-ring-alpha)), var(--ui-shadow-xl)',
      '2xl': '0 0 0 1px rgb(var(--c-rule) / var(--ui-shadow-ring-alpha)), var(--ui-shadow-2xl)',
      inner: 'var(--ui-shadow-inner)'
    },
    extend: {
      colors: {
        // 主色：新版钱绿 / 经典 teal
        primary: scale('primary'),
        // 中性色：亮色界面用 gray，暗色界面用 dark
        gray: scale('gray'),
        dark: scale('dark'),
        accent: scale('accent'),
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
        sans: 'var(--font-sans)',
        mono: 'var(--font-mono)'
      },
      boxShadow: {
        glass: 'var(--ui-shadow-glass)',
        'glass-sm': 'var(--ui-shadow-glass-sm)',
        glow: 'var(--ui-shadow-glow)',
        'glow-lg': 'var(--ui-shadow-glow-lg)',
        card: 'var(--ui-shadow-card)',
        'card-hover': 'var(--ui-shadow-card-hover)',
        'inner-glow': 'var(--ui-shadow-inner-glow)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'var(--ui-gradient-primary)',
        'gradient-dark': 'var(--ui-gradient-dark)',
        'gradient-glass': 'var(--ui-gradient-glass)',
        'mesh-gradient': 'var(--ui-mesh-gradient)'
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
        // 经典版为发光呼吸，新版为 none；glow 关键帧写在 style.css（动画值是变量，Tailwind 不会自动带出关键帧）
        glow: 'var(--ui-animate-glow)',
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
