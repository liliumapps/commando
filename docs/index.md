---
title: Commando
hide:
  - navigation
  - toc
---

<div class="landing" markdown>

<div class="hero" markdown>

<p class="hero__prompt"><span>~/work</span><b>&gt;</b><code>commando</code><i></i></p>

# Commando

<p class="hero__tagline">
The command launcher that already knows your terminal.
</p>

<p class="hero__sub">
Search cheat sheets, project commands, bookmarks, and shell history in one
fast TUI. Fill in arguments, review the result, and run it on your terms.
</p>

<div class="hero__actions" markdown>
[Install Commando :material-arrow-right:](getting-started/installation.md){ .md-button .md-button--primary }
[GitHub :fontawesome-brands-github:](https://github.com/liliumapps/commando){ .md-button }
</div>

<ul class="hero__meta">
  <li><span></span>Zsh, Bash & Fish</li>
  <li><span></span>Single Go binary</li>
  <li><span></span>Plain TOML state</li>
</ul>

</div>

<div class="termframe" markdown>
<div class="termframe__bar">
  <span class="termframe__controls" aria-hidden="true">
    <i></i><i></i><i></i>
  </span>
  <span class="termframe__title">commando / interactive session</span>
  <span class="termframe__status">TUI</span>
</div>

![Commando: search across sources, fill in an optional and multi-select command, and it lands on your prompt](assets/gifs/tour.gif)

<div class="termframe__caption">
  <span>01 Search</span>
  <span>02 Fill arguments</span>
  <span>03 Review on prompt</span>
</div>

</div>

<div class="install-line">
<span class="install-line__label">Quick install</span>
<div class="install-line__command" markdown>
```sh
curl -fsSL https://raw.githubusercontent.com/liliumapps/commando/main/install.sh | sh
```
</div>
</div>

## One launcher. Every command source.

Existing tools each solve part of the problem. `fzf` searches, cheat tools
template, history tools recall — but none of them combine those into one flow
with first-class **optional** and **multi-select** parameters, plus memory that
learns your habits. Commando is the one resolution engine behind all of it.

<div class="grid cards feature-grid" markdown>

-   <span class="feature-card__index">01</span> :material-file-document-multiple: __Cheat sheets__

    ---

    Parameterized, reusable command templates in plain TOML. Optional segments,
    multi-select, candidate lists, previews, and shared variable groups.

    [:octicons-arrow-right-24: Cheat sheets](guide/cheat-sheets.md)

-   <span class="feature-card__index">02</span> :material-folder-search: __Project commands__

    ---

    `make` targets, `npm`/`yarn`/`pnpm` scripts, `cargo`, Gradle, `just`,
    docker-compose services, and git actions — auto-detected from your folder.

    [:octicons-arrow-right-24: Project commands](guide/project-commands.md)

-   <span class="feature-card__index">03</span> :material-history: __Shell history__

    ---

    Read live from your shell's history file, fuzzy-searchable, with a
    configurable redaction denylist for secret-looking commands.

    [:octicons-arrow-right-24: Shell history](guide/shell-history.md)

-   <span class="feature-card__index">04</span> :material-brain: __Argument memory__

    ---

    Commando remembers the values you enter per variable and ranks them by
    **frecency** — recency × frequency — so hot values rise to the top.

    [:octicons-arrow-right-24: Argument memory](guide/argument-memory.md)

-   <span class="feature-card__index">05</span> :material-tune-variant: __Optional & multi params__

    ---

    Skip an optional field and its whole `[...]` fragment disappears — no
    dangling flags. Multi-select fields format and join values for you.

    [:octicons-arrow-right-24: Template syntax](guide/templates.md)

-   <span class="feature-card__index">06</span> :material-file-eye: __Plain files, no database__

    ---

    Every piece of state is a human-readable TOML file you can `cat`, `grep`,
    and hand-edit. Nothing is locked in an opaque store.

    [:octicons-arrow-right-24: Files & storage](reference/files.md)

</div>

## From prompt to command in seconds

```sh
# Install and wire up your shell (Zsh shown; Bash and Fish are supported)
curl -fsSL https://raw.githubusercontent.com/liliumapps/commando/main/install.sh | sh
echo 'eval "$(commando init zsh)"' >> ~/.zshrc && exec zsh

# Summon the launcher, or press Ctrl-G at any prompt
commando

# Search, fill arguments, then review the finished command on your prompt
```

Commando **never runs your command for you by default.** It assembles it and
hands it to your shell line editor, so `cd`, environment variables, and aliases
all keep working — and you get the last word before anything executes.

[Read the installation guide :material-arrow-right:](getting-started/installation.md){ .md-button .md-button--primary }

</div>

## More from Lilium Apps

Commando is made by [Lilium Apps](https://liliumapps.com), an independent
studio. Other things we make:

- [Lumen](https://lumen.liliumapps.com): a privacy-first Android file manager
  with fast search and storage insights.
  [Get it on Google Play](https://play.google.com/store/apps/details?id=com.liliumapps.lumenfiles).
- [VLC Stream and Remote](https://vlc.liliumapps.com): control VLC on your
  computer from your phone.
  [Get it on Google Play](https://play.google.com/store/apps/details?id=tuple.me.vlcremote).

See everything at [liliumapps.com](https://liliumapps.com).

## Credits

Commando is inspired by [navi](https://github.com/denisidoro/navi) by Denis
Isidoro, the interactive cheat sheet tool for the command line. Its cheat
sheets with variables and suggestion lists shaped the core idea here.

