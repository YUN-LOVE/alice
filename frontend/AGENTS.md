## Development

Package manager is **pnpm**（不要用 npm）。

Starting the dev server in background mode:

```
pnpm astro dev --background
```

Manage the background server with `pnpm astro dev stop`, `pnpm astro dev status`, and `pnpm astro dev logs`.

Common commands:

```
pnpm install        # 安装依赖
pnpm dev            # 开发（默认监听 0.0.0.0:4321，局域网可访问）
pnpm build          # 构建
```

If dependencies with build scripts are ignored by pnpm (`ERR_PNPM_IGNORED_BUILDS`), run `pnpm approve-builds --all`.

## Documentation

Full documentation: https://docs.astro.build

Consult these guides before working on related tasks:

- [Adding pages, dynamic routes, or middleware](https://docs.astro.build/en/guides/routing/)
- [Working with Astro components](https://docs.astro.build/en/basics/astro-components/)
- [Using React, Vue, Svelte, or other framework components](https://docs.astro.build/en/guides/framework-components/)
- [Adding or managing content](https://docs.astro.build/en/guides/content-collections/)
- [Adding styles or using Tailwind](https://docs.astro.build/en/guides/styling/)
- [Supporting multiple languages](https://docs.astro.build/en/guides/internationalization/)
