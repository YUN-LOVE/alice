// @ts-check
import { defineConfig } from 'astro/config';
import vue from '@astrojs/vue';
import tailwindcss from '@tailwindcss/vite';

// https://astro.build/config
export default defineConfig({
	integrations: [vue()],
	vite: {
		plugins: [tailwindcss()],
		define: {
			__VUE_PROD_DEVTOOLS__: false,
			__VUE_OPTIONS_API__: false,
		},
	},
});
