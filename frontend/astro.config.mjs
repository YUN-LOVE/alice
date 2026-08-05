// @ts-check
import { defineConfig } from 'astro/config';
import vue from '@astrojs/vue';
import tailwindcss from '@tailwindcss/vite';

// https://astro.build/config
export default defineConfig({
	server: {
		// 监听所有接口（局域网/多端可访问）
		host: '0.0.0.0',
	},
	integrations: [vue()],
	vite: {
		plugins: [tailwindcss()],
		define: {
			__VUE_PROD_DEVTOOLS__: false,
			__VUE_OPTIONS_API__: false,
		},
	},
});
