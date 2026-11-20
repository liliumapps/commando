(() => {
  "use strict";

  const root = document.documentElement;
  const body = document.body;
  const select = (selector, scope = document) => scope.querySelector(selector);
  const selectAll = (selector, scope = document) => [...scope.querySelectorAll(selector)];

  const setTheme = (theme) => {
    root.dataset.theme = theme;
    try {
      localStorage.setItem("commando-docs-theme", theme);
    } catch (_) {
      // Storage can be unavailable in hardened browser contexts.
    }

    const themeColor = select('meta[name="theme-color"]');
    if (themeColor) {
      themeColor.content = theme === "light" ? "#f7f9fa" : "#090c0f";
    }

    const toggle = select("[data-theme-toggle]");
    if (toggle) {
      toggle.setAttribute(
        "aria-label",
        theme === "dark" ? "Switch to light theme" : "Switch to dark theme",
      );
      toggle.title = toggle.getAttribute("aria-label");
    }
  };

  setTheme(root.dataset.theme === "light" ? "light" : "dark");
  select("[data-theme-toggle]")?.addEventListener("click", () => {
    setTheme(root.dataset.theme === "dark" ? "light" : "dark");
  });

  const drawer = select("[data-drawer]");
  const drawerOpen = select("[data-drawer-open]");
  const openDrawer = () => {
    if (!drawer) return;
    body.classList.add("is-drawer-open");
    drawer.setAttribute("aria-hidden", "false");
    drawerOpen?.setAttribute("aria-expanded", "true");
    select("[data-drawer-close]", drawer)?.focus();
  };
  const closeDrawer = () => {
    if (!drawer) return;
    body.classList.remove("is-drawer-open");
    drawer.setAttribute("aria-hidden", "true");
    drawerOpen?.setAttribute("aria-expanded", "false");
  };

  drawerOpen?.addEventListener("click", openDrawer);
  select("[data-drawer-close]")?.addEventListener("click", closeDrawer);
  select("[data-drawer-backdrop]")?.addEventListener("click", closeDrawer);
  selectAll("a", drawer).forEach((link) => link.addEventListener("click", closeDrawer));

  selectAll(".doc-content table").forEach((table) => {
    if (table.parentElement?.classList.contains("table-scroll")) return;
    const wrapper = document.createElement("div");
    wrapper.className = "table-scroll";
    table.before(wrapper);
    wrapper.append(table);
  });

  const copyIcon = select("#copy-icon");
  const checkIcon = select("#check-icon");
  selectAll(".doc-content .highlight").forEach((block) => {
    if (block.closest(".termframe") || !select("pre", block)) return;

    const button = document.createElement("button");
    button.type = "button";
    button.className = "code-copy icon-button";
    button.setAttribute("aria-label", "Copy code");
    button.title = "Copy code";
    if (copyIcon) button.append(copyIcon.content.cloneNode(true));

    button.addEventListener("click", async () => {
      const code = select("pre", block)?.innerText || "";
      try {
        await navigator.clipboard.writeText(code.replace(/\n$/, ""));
      } catch (_) {
        const area = document.createElement("textarea");
        area.value = code;
        area.style.position = "fixed";
        area.style.opacity = "0";
        body.append(area);
        area.select();
        document.execCommand("copy");
        area.remove();
      }

      button.classList.add("is-copied");
      button.setAttribute("aria-label", "Copied");
      button.replaceChildren(checkIcon?.content.cloneNode(true) || document.createTextNode("Done"));
      window.setTimeout(() => {
        button.classList.remove("is-copied");
        button.setAttribute("aria-label", "Copy code");
        button.replaceChildren(copyIcon?.content.cloneNode(true) || document.createTextNode("Copy"));
      }, 1600);
    });

    block.append(button);
  });

  const progress = select("[data-reading-progress]");
  const backToTop = select("[data-back-to-top]");
  const updateScrollState = () => {
    const scrollable = document.documentElement.scrollHeight - window.innerHeight;
    const percent = scrollable > 0 ? Math.min(100, (window.scrollY / scrollable) * 100) : 0;
    if (progress) progress.style.width = `${percent}%`;
    backToTop?.classList.toggle("is-visible", window.scrollY > 700);
  };

  window.addEventListener("scroll", updateScrollState, { passive: true });
  window.addEventListener("resize", updateScrollState);
  updateScrollState();
  backToTop?.addEventListener("click", () => window.scrollTo({ top: 0, behavior: "smooth" }));

  const tocLinks = selectAll(".page-toc a[href^='#']");
  if (tocLinks.length && "IntersectionObserver" in window) {
    const linksById = new Map(
      tocLinks.map((link) => [decodeURIComponent(link.hash.slice(1)), link]),
    );
    const headings = [...linksById.keys()]
      .map((id) => document.getElementById(id))
      .filter(Boolean);

    const activateToc = (id) => {
      tocLinks.forEach((link) => link.classList.toggle("is-active", link === linksById.get(id)));
    };

    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        if (visible[0]) activateToc(visible[0].target.id);
      },
      { rootMargin: "-20% 0px -70% 0px", threshold: 0 },
    );
    headings.forEach((heading) => observer.observe(heading));
    if (headings[0]) activateToc(headings[0].id);
  }

  const searchDialog = select("[data-search-dialog]");
  const searchInput = select("[data-search-input]");
  const searchResults = select("[data-search-results]");
  const searchEmpty = select("[data-search-empty]");
  let searchDocuments = null;
  let selectedResult = -1;

  const htmlToText = (html) => {
    const template = document.createElement("template");
    template.innerHTML = html || "";
    return (template.content.textContent || "").replace(/\s+/g, " ").trim();
  };

  const escapeRegExp = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

  const appendHighlighted = (element, text, terms) => {
    if (!terms.length) {
      element.textContent = text;
      return;
    }

    const expression = new RegExp(`(${terms.map(escapeRegExp).join("|")})`, "gi");
    text.split(expression).forEach((part) => {
      if (terms.some((term) => part.toLowerCase() === term)) {
        const mark = document.createElement("mark");
        mark.textContent = part;
        element.append(mark);
      } else {
        element.append(document.createTextNode(part));
      }
    });
  };

  const siteRoot = new URL(root.dataset.siteRoot || "./", window.location.href);
  const resultUrl = (location) => new URL(location || "./", siteRoot).href;

  const scoreDocument = (documentEntry, terms) => {
    const title = documentEntry.cleanTitle.toLowerCase();
    const text = documentEntry.cleanText.toLowerCase();
    const location = documentEntry.location.toLowerCase();
    let score = 0;

    for (const term of terms) {
      if (!title.includes(term) && !text.includes(term) && !location.includes(term)) return -1;
      if (title === term) score += 30;
      else if (title.startsWith(term)) score += 18;
      else if (title.includes(term)) score += 10;
      if (location.includes(term)) score += 4;
      if (text.includes(term)) score += 2;
    }
    return score;
  };

  const snippetFor = (text, terms) => {
    const lower = text.toLowerCase();
    const positions = terms.map((term) => lower.indexOf(term)).filter((position) => position >= 0);
    const first = positions.length ? Math.min(...positions) : 0;
    const start = Math.max(0, first - 65);
    const end = Math.min(text.length, start + 165);
    return `${start > 0 ? "..." : ""}${text.slice(start, end).trim()}${end < text.length ? "..." : ""}`;
  };

  const setSelectedResult = (index) => {
    const results = selectAll(".search-result", searchResults);
    if (!results.length) {
      selectedResult = -1;
      return;
    }

    selectedResult = (index + results.length) % results.length;
    results.forEach((result, resultIndex) => {
      const selected = resultIndex === selectedResult;
      result.classList.toggle("is-selected", selected);
      result.setAttribute("aria-selected", selected ? "true" : "false");
    });
    results[selectedResult]?.scrollIntoView({ block: "nearest" });
  };

  const renderSearch = (query) => {
    if (!searchResults || !searchEmpty) return;
    const terms = query.toLowerCase().trim().split(/\s+/).filter(Boolean);
    searchResults.replaceChildren();
    selectedResult = -1;

    if (!terms.length) {
      searchEmpty.hidden = false;
      return;
    }

    if (!searchDocuments) {
      searchEmpty.hidden = false;
      select("p", searchEmpty).textContent = "Loading search index...";
      select("small", searchEmpty).textContent = "";
      return;
    }

    const matches = searchDocuments
      .map((documentEntry) => ({
        documentEntry,
        score: scoreDocument(documentEntry, terms),
      }))
      .filter((match) => match.score >= 0)
      .sort((a, b) => b.score - a.score || a.documentEntry.cleanTitle.localeCompare(b.documentEntry.cleanTitle))
      .slice(0, 12);

    searchEmpty.hidden = matches.length > 0;
    if (!matches.length) {
      select("p", searchEmpty).textContent = `No results for "${query.trim()}"`;
      select("small", searchEmpty).textContent = "Try a shorter query or a different command name.";
      return;
    }

    matches.forEach(({ documentEntry }) => {
      const link = document.createElement("a");
      link.className = "search-result";
      link.href = resultUrl(documentEntry.location);
      link.setAttribute("role", "option");
      link.setAttribute("aria-selected", "false");

      const title = document.createElement("strong");
      appendHighlighted(title, documentEntry.cleanTitle, terms);
      const snippet = document.createElement("small");
      appendHighlighted(snippet, snippetFor(documentEntry.cleanText, terms), terms);
      const path = document.createElement("em");
      path.textContent = documentEntry.location || "Home";

      link.append(title, snippet, path);
      searchResults.append(link);
    });
    setSelectedResult(0);
  };

  const loadSearch = async () => {
    if (searchDocuments) return;
    try {
      const response = await fetch(root.dataset.searchIndex);
      if (!response.ok) throw new Error(`Search index returned ${response.status}`);
      const data = await response.json();
      searchDocuments = data.docs.map((documentEntry) => ({
        ...documentEntry,
        cleanTitle: htmlToText(documentEntry.title),
        cleanText: htmlToText(documentEntry.text),
      }));
      renderSearch(searchInput?.value || "");
    } catch (_) {
      if (searchEmpty) {
        searchEmpty.hidden = false;
        select("p", searchEmpty).textContent = "Search is unavailable";
        select("small", searchEmpty).textContent = "Reload the page and try again.";
      }
    }
  };

  const openSearch = () => {
    if (!searchDialog) return;
    closeDrawer();
    if (typeof searchDialog.showModal === "function") searchDialog.showModal();
    else searchDialog.setAttribute("open", "");
    body.classList.add("is-search-open");
    loadSearch();
    window.requestAnimationFrame(() => searchInput?.focus());
  };

  const closeSearch = () => {
    if (!searchDialog?.open) return;
    searchDialog.close();
    body.classList.remove("is-search-open");
  };

  selectAll("[data-search-open]").forEach((button) => button.addEventListener("click", openSearch));
  select("[data-search-close]")?.addEventListener("click", closeSearch);
  searchDialog?.addEventListener("close", () => body.classList.remove("is-search-open"));
  searchDialog?.addEventListener("click", (event) => {
    if (event.target === searchDialog) closeSearch();
  });
  searchInput?.addEventListener("input", (event) => renderSearch(event.target.value));
  searchInput?.addEventListener("keydown", (event) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelectedResult(selectedResult + 1);
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelectedResult(selectedResult - 1);
    } else if (event.key === "Enter" && selectedResult >= 0) {
      event.preventDefault();
      selectAll(".search-result", searchResults)[selectedResult]?.click();
    }
  });

  document.addEventListener("keydown", (event) => {
    const target = event.target;
    const isTyping =
      target instanceof HTMLInputElement ||
      target instanceof HTMLTextAreaElement ||
      target?.isContentEditable;

    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      openSearch();
    } else if (event.key === "/" && !isTyping && !searchDialog?.open) {
      event.preventDefault();
      openSearch();
    } else if (event.key === "Escape") {
      closeDrawer();
      closeSearch();
    }
  });
})();
