# Atlas API, Internals, and Troubleshooting

Reference material for the Salesforce developer docs scraper. Read this when
debugging, exploring an unfamiliar deliverable, or scraping a subsection.

## How it works

Salesforce developer docs use the **Atlas** framework, which delivers content
through a JSON API even though the page chrome is rendered with JavaScript. The
scraper never renders JavaScript; it calls the API directly:

1. Use `curl-cffi` (bundles libcurl-impersonate) to spoof Chrome's TLS
   fingerprint and get past the Akamai WAF.
2. Fetch the TOC from the `get_document/atlas.*.meta` endpoint.
3. Fetch each page's content from the `get_document_content` endpoint.
4. Convert HTML to markdown with `markdownify`, preserving tables and rewriting
   internal links.

## Key API endpoints

| Endpoint                                                           | Purpose                                                     |
| ------------------------------------------------------------------ | ----------------------------------------------------------- |
| `get_document/atlas.{locale}.{deliverable}.meta`                   | Full TOC + metadata (includes `version.doc_version`).       |
| `get_document_content/{deliverable}/{page}.htm/{locale}/{version}` | Single page content (JSON with `title` and `content` HTML). |

Base URL: `https://developer.salesforce.com/docs`

The TOC response contains:

- `toc`: tree of nodes with `id`, `text` (title), `a_attr.href`, `children`.
- `version.doc_version`: version number required by the content endpoint.
- `deliverable`: the deliverable name.

## Manual fetch commands (debugging)

Use these to inspect the raw API responses without running the full scraper.

Fetch the TOC:

```bash
uv run --with curl-cffi python3 -c "
from curl_cffi import requests
r = requests.get(
    'https://developer.salesforce.com/docs/get_document/atlas.en-us.{DELIVERABLE}.meta',
    impersonate='chrome',
)
print(r.text)
" > toc.json
```

Fetch a single page's content (needs the `doc_version` from the TOC):

```bash
uv run --with curl-cffi python3 -c "
from curl_cffi import requests
r = requests.get(
    'https://developer.salesforce.com/docs/get_document_content/{DELIVERABLE}/{PAGE}.htm/en-us/{VERSION}',
    impersonate='chrome',
)
print(r.text)
" > page.json
```

## TOC parsing internals

Many TOC entries are `#anchor` fragments pointing to sections within the same
page. The scraper dedupes by the `.htm` filename so each page is fetched once:

```python
def extract_pages(root_node):
    """Recursively extract unique pages from a TOC subtree."""
    pages = []
    seen = set()

    def walk(node, path_parts):
        href = node.get('a_attr', {}).get('href', '')
        htm = href.split('#')[0] if href else ''

        if htm and htm not in seen:
            seen.add(htm)
            pages.append({
                'id': node.get('id'),
                'htm': htm,
                'title': node['text'],
                'path_parts': list(path_parts),
                'slug': slugify(node['text']),
            })

        for child in node.get('children', []):
            walk(child, path_parts + [slugify(node['text'])])

    walk(root_node, [])
    return pages
```

## Scraping a subsection

To scrape only part of a larger guide (e.g. "Product Discovery" from a big dev
guide):

1. Fetch the full TOC.
2. Find the target node by `id` (e.g. `product_discovery_overview`).
3. Pass `--root-id product_discovery_overview` to the scraper. It extracts and
   fetches only that subtree.

```bash
uv run --with curl-cffi --with markdownify \
  python3 skills/salesforce-docs-scrape/scripts/scraper.py \
  --url "https://developer.salesforce.com/docs/atlas.en-us.apexcode.meta/apexcode/apex_reference.htm" \
  --root-id "apex_classes_restful_http" \
  --output ./docs/http-classes
```

## Troubleshooting

### WAF blocks (403 or empty response)

- Ensure `curl-cffi` is used with `impersonate="chrome"`, not plain `curl` or
  `requests`. The WAF checks the TLS fingerprint (JA3/JA4), not cookies/headers.

### Empty JSON content

- The scraper already retries with backoff. If a page keeps returning empty
  content, add more delay between requests (the default is 400 ms).

### Missing pages

- Some TOC nodes are `#anchor` fragments, not standalone pages. The scraper
  dedupes by `.htm` filename, so these collapse into their parent page — this is
  expected, not a bug.
