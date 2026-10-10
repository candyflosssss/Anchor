import type { Config } from 'tailwindcss';
import animate from 'tailwindcss-animate';
export default {
  darkMode: ['class'],
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: { extend: {
    colors: Object.fromEntries(['background', 'foreground', 'primary', 'primary-foreground', 'secondary', 'secondary-foreground', 'muted', 'muted-foreground', 'accent', 'accent-foreground', 'border', 'input', 'ring', 'destructive', 'destructive-foreground', 'popover', 'popover-foreground', 'card', 'card-foreground', 'sidebar', 'sidebar-foreground', 'sidebar-primary', 'sidebar-primary-foreground', 'sidebar-accent', 'sidebar-accent-foreground', 'sidebar-border', 'sidebar-ring'].map(name => [name, `hsl(var(--${name}) / <alpha-value>)`])),
    borderRadius: { lg: 'var(--radius)', md: 'calc(var(--radius) - 2px)', sm: 'calc(var(--radius) - 4px)' },
  } },
  plugins: [animate],
} satisfies Config;
