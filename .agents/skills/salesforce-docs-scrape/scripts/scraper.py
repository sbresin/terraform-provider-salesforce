#!/usr/bin/env python3
"""
Salesforce Atlas Documentation Scraper

Scrapes Salesforce developer documentation to local markdown files.
Uses curl-cffi to bypass Akamai WAF via TLS fingerprint spoofing.

Works only for developer.salesforce.com/docs (Atlas) pages.

Usage:
    uv run --with curl-cffi --with markdownify python3 \
        skills/salesforce-docs-scrape/scripts/scraper.py \
        --url "https://developer.salesforce.com/docs/atlas.en-us.DELIVERABLE.meta/DELIVERABLE/PAGE.htm" \
        --output ./docs

    # Or scrape a specific subsection:
    uv run --with curl-cffi --with markdownify python3 \
        skills/salesforce-docs-scrape/scripts/scraper.py \
        --url "https://developer.salesforce.com/docs/atlas.en-us.DELIVERABLE.meta/DELIVERABLE/PAGE.htm" \
        --root-id "specific_section_id" \
        --output ./docs
"""

import argparse
import re
import sys
import time
from pathlib import Path
from urllib.parse import urlparse

from curl_cffi import requests
from markdownify import MarkdownConverter

# === Constants ===
DELAY = 0.4  # seconds between requests
MAX_RETRIES = 3
BASE_API_URL = "https://developer.salesforce.com/docs"


def slugify(text: str) -> str:
    """Convert text to kebab-case slug."""
    s = text.lower()
    s = re.sub(r"[^a-z0-9]+", "-", s)
    s = s.strip("-")
    return s


class SalesforceConverter(MarkdownConverter):
    """Custom markdown converter for Salesforce HTML."""

    def __init__(self, link_map: dict, **kwargs):
        super().__init__(**kwargs)
        self.link_map = link_map

    def convert_a(self, el, text, parent_tags):
        """Rewrite internal .htm links to relative .md paths."""
        href = el.get("href", "")
        title = el.get("title", "")

        if not href or not text.strip():
            return text or ""

        if ".htm" in href:
            parsed = urlparse(href)
            path = parsed.path
            anchor = parsed.fragment

            htm_file = path.split("/")[-1] if "/" in path else path

            if htm_file in self.link_map:
                new_path = self.link_map[htm_file]
                if anchor:
                    new_path = f"{new_path}#{anchor}"
                href = new_path

        if title:
            return f'[{text}]({href} "{title}")'
        return f"[{text}]({href})"

    def convert_samp(self, el, text, parent_tags):
        """Convert <samp> to inline code."""
        return f"`{text}`" if text else ""

    def convert_pre(self, el, text, parent_tags):
        """Preserve code blocks."""
        code_el = el.find("code")
        lang = ""
        if code_el:
            classes = code_el.get("class", [])
            if isinstance(classes, str):
                classes = classes.split()
            for c in classes:
                if c.startswith("language-"):
                    lang = c.replace("language-", "")
                    break

        code_text = el.get_text()
        return f"\n```{lang}\n{code_text}\n```\n"


class SalesforceScraper:
    """Scraper for Salesforce Atlas documentation."""

    def __init__(self, output_dir: Path, verbose: bool = True):
        self.output_dir = output_dir
        self.verbose = verbose
        self.session = requests.Session(impersonate="chrome")
        self.pages = []
        self.link_map = {}
        self.deliverable = None
        self.doc_version = None
        self.locale = "en-us"

    def log(self, msg: str):
        if self.verbose:
            print(msg, flush=True)

    def parse_url(self, url: str) -> tuple[str, str, str]:
        """Parse Salesforce doc URL to extract deliverable, locale, and page ID."""
        # URL format: .../atlas.{locale}.{deliverable}.meta/{deliverable}/{page}.htm
        match = re.search(r"atlas\.([a-z-]+)\.([^.]+)\.meta/([^/]+)/([^.]+)\.htm", url)
        if not match:
            raise ValueError(f"Cannot parse Salesforce doc URL: {url}")

        locale, deliverable, _, page_id = match.groups()
        return deliverable, locale, page_id

    def fetch_toc(self, deliverable: str, locale: str) -> dict:
        """Fetch the full documentation TOC."""
        url = f"{BASE_API_URL}/get_document/atlas.{locale}.{deliverable}.meta"
        self.log(f"Fetching TOC from {url}...")

        resp = self.session.get(url, timeout=60)
        if resp.status_code != 200:
            raise RuntimeError(f"Failed to fetch TOC: HTTP {resp.status_code}")

        return resp.json()

    def find_node(self, nodes: list, target_id: str) -> dict | None:
        """Find a TOC node by ID using iterative DFS (avoids Python's
        recursion limit on very deep/large Atlas TOC trees)."""
        stack = list(nodes)
        while stack:
            node = stack.pop()
            if node.get("id") == target_id:
                return node
            children = node.get("children")
            if children:
                stack.extend(children)
        return None

    def extract_pages(self, root_node: dict) -> list[dict]:
        """Extract unique pages from TOC subtree."""
        pages = []
        seen = set()

        def walk(node, path_parts):
            href = node.get("a_attr", {}).get("href", "")
            htm = href.split("#")[0] if href else ""

            if htm and htm not in seen:
                seen.add(htm)
                pages.append(
                    {
                        "id": node.get("id", ""),
                        "htm": htm,
                        "title": node["text"],
                        "path_parts": list(path_parts),
                        "slug": slugify(node["text"]),
                    }
                )

            for child in node.get("children", []):
                walk(child, path_parts + [slugify(node["text"])])

        walk(root_node, [])
        return pages

    def build_link_map(self) -> dict[str, str]:
        """Build mapping from htm filename to relative md path."""
        link_map = {}
        for p in self.pages:
            parts = p["path_parts"] + [p["slug"]]
            rel_path = "/".join(parts) + ".md"
            link_map[p["htm"]] = rel_path
        return link_map

    def fetch_page(self, htm: str) -> dict | None:
        """Fetch a single page with retries."""
        url = f"{BASE_API_URL}/get_document_content/{self.deliverable}/{htm}/{self.locale}/{self.doc_version}"

        for attempt in range(MAX_RETRIES):
            try:
                resp = self.session.get(url, timeout=30)
                if resp.status_code == 200:
                    data = resp.json()
                    if data.get("content"):
                        return data
                    else:
                        self.log(f"  [!] Empty content, retry {attempt + 1}")
                else:
                    self.log(f"  [!] HTTP {resp.status_code}, retry {attempt + 1}")
            except Exception as e:
                self.log(f"  [!] Error: {e}, retry {attempt + 1}")

            time.sleep(1 + attempt)

        return None

    def html_to_markdown(self, html: str, title: str, source_url: str) -> str:
        """Convert Salesforce HTML to markdown."""
        # Remove anchor tags at start
        html = re.sub(r'<a name="[^"]*"><!--\s*--></a>', "", html)

        converter = SalesforceConverter(
            link_map=self.link_map,
            heading_style="atx",
            bullets="-",
            strip=["script", "style"],
        )

        content = converter.convert(html)
        content = re.sub(r"\n{3,}", "\n\n", content)
        content = content.strip()

        return f"# {title}\n\n> Source: [{source_url}]({source_url})\n\n{content}"

    def build_readme(self) -> str:
        """Build README.md index with nested TOC."""
        lines = [
            f"# {self.pages[0]['title'] if self.pages else 'Documentation'}",
            "",
            "Salesforce Developer Documentation",
            "",
            "## Contents",
            "",
        ]

        for p in self.pages:
            indent = "  " * len(p["path_parts"])
            parts = p["path_parts"] + [p["slug"]]
            rel_path = "/".join(parts) + ".md"
            lines.append(f"{indent}- [{p['title']}]({rel_path})")

        return "\n".join(lines)

    def scrape(self, url: str, root_id: str | None = None):
        """Main scrape workflow."""
        # Parse URL
        self.deliverable, self.locale, default_root_id = self.parse_url(url)
        root_id = root_id or default_root_id

        self.log(f"Deliverable: {self.deliverable}")
        self.log(f"Locale: {self.locale}")
        self.log(f"Root ID: {root_id}")

        # Fetch TOC
        toc_data = self.fetch_toc(self.deliverable, self.locale)
        self.doc_version = toc_data["version"]["doc_version"]
        self.log(f"Doc version: {self.doc_version}")

        # Find root node
        root_node = self.find_node(toc_data["toc"], root_id)
        if not root_node:
            raise ValueError(f"Cannot find TOC node with ID: {root_id}")

        # Extract pages
        self.pages = self.extract_pages(root_node)
        self.log(f"Found {len(self.pages)} unique pages")

        # Build link map
        self.link_map = self.build_link_map()

        # Create output directory
        self.output_dir.mkdir(parents=True, exist_ok=True)

        # Fetch and convert pages
        source_url_base = f"https://developer.salesforce.com/docs/atlas.{self.locale}.{self.deliverable}.meta/{self.deliverable}"

        for i, p in enumerate(self.pages):
            htm = p["htm"]
            self.log(f"[{i + 1}/{len(self.pages)}] {htm}...")

            data = self.fetch_page(htm)
            if not data:
                self.log("  FAILED")
                continue

            title = data.get("title", p["title"])
            html_content = data.get("content", "")

            if not html_content:
                self.log("  SKIP (empty)")
                continue

            source_url = f"{source_url_base}/{htm}"
            markdown = self.html_to_markdown(html_content, title, source_url)

            # Write file
            folder = self.output_dir
            if p["path_parts"]:
                folder = self.output_dir / "/".join(p["path_parts"])
            folder.mkdir(parents=True, exist_ok=True)

            out_file = folder / f"{p['slug']}.md"
            out_file.write_text(markdown, encoding="utf-8")
            self.log(f"  -> {out_file.relative_to(self.output_dir)}")

            time.sleep(DELAY)

        # Write README
        readme = self.build_readme()
        (self.output_dir / "README.md").write_text(readme, encoding="utf-8")
        self.log("\nWrote README.md")

        # Summary
        md_count = len(list(self.output_dir.rglob("*.md")))
        self.log(f"\nDone! {md_count} markdown files in {self.output_dir}")


def main():
    parser = argparse.ArgumentParser(
        description="Scrape Salesforce Atlas documentation to markdown",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  # Scrape starting from a specific page (and all its children):
  %(prog)s --url "https://developer.salesforce.com/docs/atlas.en-us.revenue_lifecycle_management_dev_guide.meta/revenue_lifecycle_management_dev_guide/product_discovery_overview.htm" --output ./docs

  # Scrape a specific section by ID:
  %(prog)s --url "https://developer.salesforce.com/docs/atlas.en-us.apexcode.meta/apexcode/apex_reference.htm" --root-id "apex_classes_restful_http" --output ./docs/http-classes
        """,
    )
    parser.add_argument("--url", required=True, help="Salesforce documentation URL")
    parser.add_argument(
        "--output", "-o", required=True, help="Output directory for markdown files"
    )
    parser.add_argument(
        "--root-id", help="Override root TOC node ID (default: extracted from URL)"
    )
    parser.add_argument(
        "--quiet", "-q", action="store_true", help="Suppress progress output"
    )

    args = parser.parse_args()

    scraper = SalesforceScraper(output_dir=Path(args.output), verbose=not args.quiet)

    try:
        scraper.scrape(args.url, args.root_id)
    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


if __name__ == "__main__":
    main()
