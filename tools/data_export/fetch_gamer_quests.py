#!/usr/bin/env python3
"""Cache Bahamut's public WLO quest references and export a factual section index."""
import argparse
import csv
import hashlib
from html.parser import HTMLParser
import json
from pathlib import Path
import re
import time
from urllib.parse import parse_qs, urlencode, urljoin, urlsplit
from urllib.request import Request, urlopen

MENU_URL = 'https://acg.gamer.com.tw/wikimenu.php?s=113640'
PAGE_BASE = 'https://wiki2.gamer.com.tw/wiki.php'
NAMESPACE = '9045:'
PORTAL_ID = '113640'
REQUEST_INTERVAL_SECONDS = 1
SUPPORT_PAGES = {'魯賓遜新手島', '寵物NPC', '結婚系統', '女神技能', '真女神技',
                 '龍魂技', '職業技能', '魂閃', '箭雨', '生活技能', '煉金術'}
FIELD_PATTERNS = {
    'location': r'(?:觸發地點|觸發地圖|任務地點|地點|地圖)',
    'npc': r'(?:觸發人物|觸發對象|觸發物品|觸發NPC|觸發對像|任務NPC)',
    'prerequisites': r'(?:任務限制|觸發條件|任務條件|前置任務|限制|條件|所需道具)',
    'required_items': r'(?:任務需品|任務需求|需求道具|需備物品|消耗道具|所需物品)',
    'rewards': r'(?:任務獎勵|獎勵|報酬)',
}


def page_url(title):
    return PAGE_BASE + '?' + urlencode({'n': NAMESPACE + title, 'ss': PORTAL_ID})


def linked_title(href):
    url = urlsplit(urljoin(PAGE_BASE, href))
    if url.hostname != 'wiki2.gamer.com.tw' or url.path != '/wiki.php':
        return None
    name = parse_qs(url.query).get('n', [''])[0]
    return name[len(NAMESPACE):] if name.startswith(NAMESPACE) else None


class WikiParser(HTMLParser):
    """Keep article content separate from the repeated full-site navigation."""
    def __init__(self, menu=False):
        super().__init__(convert_charrefs=True)
        self.menu = menu
        self.depth = 0
        self.target = None
        self.skip = 0
        self.link = None
        self.links = []
        self.sections = []
        self.section = dict(title='', anchor='', level=0, text=[])
        self.heading = None
        self.anchor = ''

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag == 'div':
            self.depth += 1
            if 'wikiContent' in attrs.get('class', '').split():
                self.target = self.depth
        if not self.menu and self.target is None:
            return
        if tag in ('script', 'style'):
            self.skip += 1
        if self.skip:
            return
        if tag == 'a':
            self.link = [attrs.get('href', ''), '']
            if attrs.get('name'):
                self.anchor = attrs['name']
        if re.fullmatch('h[1-6]', tag):
            self.sections.append(self.section)
            self.section = dict(title='', anchor=self.anchor, level=int(tag[1]), text=[])
            self.heading = []
        if tag in ('br', 'tr', 'div', 'li'):
            self.section['text'].append('\n')
        if tag in ('td', 'th'):
            self.section['text'].append(' | ')

    def handle_data(self, value):
        if (not self.menu and self.target is None) or self.skip:
            return
        self.section['text'].append(value)
        if self.link is not None:
            self.link[1] += value
        if self.heading is not None:
            self.heading.append(value)

    def handle_endtag(self, tag):
        active = self.menu or self.target is not None
        if active:
            if tag in ('script', 'style') and self.skip:
                self.skip -= 1
            if tag == 'a' and self.link is not None:
                self.links.append(self.link)
                self.link = None
            if re.fullmatch('h[1-6]', tag) and self.heading is not None:
                self.section['title'] = ' '.join(''.join(self.heading).split())
                self.heading = None
                self.section['text'].append('\n')
        if tag == 'div':
            if self.depth == self.target:
                self.target = None
            self.depth -= 1

    def finish(self):
        return self.sections + [self.section]


def parse(raw, menu=False):
    parser = WikiParser(menu)
    parser.feed(raw.decode('utf-8', errors='replace'))
    return parser


def factual_fields(section):
    fields = {}
    lines = [' '.join(line.split()).strip(' |') for line in ''.join(section['text']).splitlines()]
    for field, pattern in FIELD_PATTERNS.items():
        matches = []
        for line in lines:
            match = re.match(r'^' + pattern + r'\s*[:：]\s*(.+)$', line, re.I)
            if match:
                matches.append(match[1].strip(' |'))
        fields[field] = ' / '.join(dict.fromkeys(matches))
    return fields


def write_csv(path, rows, fields):
    with path.open('w', encoding='utf-8', newline='') as out:
        writer = csv.DictWriter(out, fieldnames=fields, lineterminator='\n')
        writer.writeheader()
        writer.writerows(rows)


def fetch(cache, filename, url, offline):
    path = cache / filename
    if not path.exists():
        if offline:
            raise FileNotFoundError(path)
        with urlopen(Request(url, headers={'User-Agent': 'GonlineQuestResearch/1.0'}), timeout=45) as response:
            raw = response.read()
            if response.status != 200:
                raise ValueError(f'{response.status}: {url}')
        path.write_bytes(raw)
        time.sleep(REQUEST_INTERVAL_SECONDS)
    return path.read_bytes()


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--cache', type=Path, default=Path('var/quest-research/gamer'))
    p.add_argument('--output-dir', type=Path, default=Path('var/quest-research/gamer/reference'))
    p.add_argument('--offline', action='store_true')
    p.add_argument('--extra-title', action='append', default=[])
    p.add_argument('--follow-context', action='store_true', help='also cache one hop of article links from the quest/support roots')
    args = p.parse_args()
    args.cache.mkdir(parents=True, exist_ok=True)
    args.output_dir.mkdir(parents=True, exist_ok=True)
    menu = fetch(args.cache, 'menu.html', MENU_URL, args.offline)
    menu_titles = {linked_title(href) for href, _ in parse(menu, menu=True).links}
    menu_titles.discard(None)
    roots = {title for title in menu_titles if '任務' in title} | (SUPPORT_PAGES & menu_titles)
    titles = sorted(roots | set(args.extra_title))
    queued = set(titles)
    pages, sections, sources, edges = [], [], [], []
    for title in titles:
        filename = hashlib.sha256(title.encode()).hexdigest()[:16] + '.html'
        url = page_url(title)
        try:
            raw = fetch(args.cache, filename, url, args.offline)
        except Exception as exc:
            pages.append(dict(title=title, url=url, status='fetch-failed', sections=0, fact_sections=0, bytes=0))
            print(f'FAILED {title}: {exc}', flush=True)
            continue
        parsed = parse(raw)
        all_sections = parsed.finish()
        article = [section for section in all_sections if section['title']]
        fact_count = 0
        for section in all_sections:
            facts = factual_fields(section)
            if not any(facts.values()):
                continue
            fact_count += 1
            sections.append(dict(page=title, section=section['title'] or title, heading_level=section['level'],
                                 source_url=url + ('#' + section['anchor'] if section['anchor'] else ''),
                                 **facts, status='source-facts-unmapped'))
        for href, label in parsed.links:
            target = linked_title(href)
            if target and not href.startswith('#'):
                edges.append(dict(page=title, target=target, label=' '.join(label.split()), url=page_url(target)))
                if args.follow_context and title in roots and target not in queued:
                    titles.append(target)
                    queued.add(target)
        article_text = ''.join(''.join(section['text']) for section in all_sections).strip()
        status = 'article' if article_text else 'no-article-content'
        if not fact_count and article_text.count('請輸入文字') >= 3:
            status = 'placeholder'
        pages.append(dict(title=title, url=url, status=status, sections=len(article), fact_sections=fact_count, bytes=len(raw)))
        sources.append(dict(title=title, file=filename, url=url, sha256=hashlib.sha256(raw).hexdigest(), bytes=len(raw),
                            utf8_replacement_characters=raw.decode('utf-8', 'replace').count('\ufffd')))
        # Full source text stays in the ignored cache, for subsequent comparison.
        (args.cache / (filename + '.sections.json')).write_text(json.dumps(all_sections, ensure_ascii=False, indent=2), encoding='utf-8', newline='\n')
        print(f'{title}: {len(article)} headings, {fact_count} factual sections', flush=True)
    write_csv(args.output_dir / 'pages.csv', pages, ['title', 'url', 'status', 'sections', 'fact_sections', 'bytes'])
    write_csv(args.output_dir / 'quest_sections.csv', sections, ['page', 'section', 'heading_level', 'source_url', *FIELD_PATTERNS, 'status'])
    unique_edges = [dict(zip(['page', 'target', 'label', 'url'], values)) for values in sorted({tuple(edge.values()) for edge in edges})]
    write_csv(args.output_dir / 'article_links.csv', unique_edges, ['page', 'target', 'label', 'url'])
    manifest = dict(menu_url=MENU_URL, portal_id=PORTAL_ID, namespace=NAMESPACE, menu_sha256=hashlib.sha256(menu).hexdigest(),
                    page_count=len(pages), fetched_pages=len(sources), fact_sections=len(sections),
                    roots=sorted(roots), extra_titles=sorted(set(args.extra_title)),
                    context_titles=sorted(queued-roots-set(args.extra_title)), sources=sources)
    (args.output_dir / 'sources.json').write_text(json.dumps(manifest, ensure_ascii=False, indent=2)+'\n', encoding='utf-8', newline='\n')
    print(f'Summary: {len(sources)}/{len(pages)} pages; {len(sections)} factual sections; {len(unique_edges)} links')


if __name__ == '__main__':
    main()
