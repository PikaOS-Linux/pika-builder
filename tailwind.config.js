/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['**/*.{html,templ}'],
  theme: {
    extend: {
      backgroundImage: {
        'logo': "url('../assets/images/logo.svg')",
      }
    },
  },
  plugins: [require('@tailwindcss/typography'), require('daisyui')],
  daisyui: {
    themes: ["light", "dark", {
      pika: {
        "primary": "#ffde34",
        "secondary": "#f59e0b",     
        "accent": "#06b6d4",    
        "neutral": "#6b7280",    
        "base-100": "#333333",
        "info": "#bae6fd",     
        "success": "#6ee7b7",     
        "warning": "#f9a8d4",    
        "error": "#ef4444",
      },
    }],
    base: true, // applies background color and foreground color for root element by default
    styled: true, // include daisyUI colors and design decisions for all components
    utils: true, // adds responsive and modifier utility classes
  },
}
