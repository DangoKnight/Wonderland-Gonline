#!/usr/bin/env python3
"""Cache public wiki quest-index/category/article API responses for offline research."""
import argparse
import json
from pathlib import Path
import time
from urllib.parse import urlencode
from urllib.request import Request, urlopen

from extract_wiki_quests import notepad_index

API = 'https://wonderlandonline.fandom.com/api.php'
ARTICLE_BATCH_SIZE = 20
REQUEST_INTERVAL_SECONDS = 1


def fetch(params):
    url = API + '?' + urlencode(dict(format='json', formatversion=2, **params))
    with urlopen(Request(url, headers={'User-Agent': 'GonlineQuestResearch/1.0'}), timeout=45) as response:
        result = json.load(response)
    if 'error' in result:
        raise ValueError(result['error'])
    return result


def cached(path, params):
    if path.exists():
        return json.loads(path.read_text(encoding='utf-8'))
    value = fetch(params)
    path.write_text(json.dumps(value, ensure_ascii=False), encoding='utf-8', newline='\n')
    time.sleep(REQUEST_INTERVAL_SECONDS)
    return value


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--cache', type=Path, required=True)
    a = p.parse_args()
    a.cache.mkdir(parents=True, exist_ok=True)
    index = cached(a.cache / 'notepad-1.txt', dict(action='parse', page='Quests_by_Notepad_Page', prop='text'))
    category = cached(a.cache / 'category.json', dict(action='query', list='categorymembers', cmtitle='Category:Quests', cmnamespace=0, cmlimit=500))
    if 'continue' in category:
        raise ValueError('category pagination needs explicit continuation before generating a complete inventory')
    titles = sorted({r['wiki_title'] for r in notepad_index(index)} | {r['title'] for r in category['query']['categorymembers']})
    for offset in range(0, len(titles), ARTICLE_BATCH_SIZE):
        number = offset // ARTICLE_BATCH_SIZE
        result = cached(a.cache / f'batch-{number:02}.json', dict(action='query', prop='revisions', rvprop='ids|timestamp|content', rvslots='main', redirects=1, titles='|'.join(titles[offset:offset + ARTICLE_BATCH_SIZE])))
        print(f'Batch {number}: {len(result["query"]["pages"])} pages', flush=True)


if __name__ == '__main__':
    main()
