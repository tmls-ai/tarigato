"""Check static assets, documentation links, and translation parity with stdlib."""
from pathlib import Path
from html.parser import HTMLParser
from urllib.parse import urlsplit, parse_qs, unquote
import json
import re

root = Path(__file__).resolve().parents[1] / 'dist'
class Page(HTMLParser):
    def __init__(self, text):
        super().__init__(); self.urls = []; self.ids = set(); self.feed(text)
    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if 'id' in attrs: self.ids.add(attrs['id'])
        self.urls += [attrs[a] for a in ['href', 'src'] if a in attrs]

for file in root.rglob('*'):
    if file.suffix in ['.html','.json','.js']:
        assert '\u2014' not in file.read_text(), f'Em dash in {file.name}'
assert not re.search(r'(?:box|text)-shadow\s*:|drop-shadow\(', (root/'style.css').read_text()), 'Shadow styling is not allowed'

for asset in re.findall(r'url\(([^)]+)\)', (root/'style.css').read_text()):
    assert (root/asset).is_file(), f'Missing CSS asset: {asset}'

english = json.loads((root/'locales/en.json').read_text())
for locale in ['en', 'ja', 'zh-CN', 'de']:
    copy = json.loads((root/'locales'/f'{locale}.json').read_text())
    assert copy.keys() == english.keys(), f'Translation keys differ: {locale}'
    assert all(isinstance(v,str) and v.strip() for v in copy.values())
    ids = Page((root/'content'/f'quickstart.{locale}.html').read_text()).ids
    assert {'install','usage','how-it-works','example','results','limits'} <= ids
for file in root.rglob('*.html'):
    page = Page(file.read_text())
    for value in page.urls:
        url = urlsplit(value)
        if url.scheme or url.netloc or not url.path: continue
        target = root / unquote(url.path).lstrip('/')
        if url.path in ['.', './', '/']: target = root/'index.html'
        assert target.is_file(), f'{file.name}: missing {url.path}'
        if target.name == 'docs.html' and url.fragment:
            params = parse_qs(url.query)
            article = params.get('article',['quickstart'])[0]
            locale = params.get('lang',['en'])[0] if article == 'quickstart' else 'en'
            assert unquote(url.fragment) in Page((root/'content'/f'{article}.{locale}.html').read_text()).ids, value
    for key in re.findall(r'data-i18n="([^"]+)"', file.read_text()):
        assert key in english, f'Unknown copy key: {key}'
print('All static links, article anchors, and 4 translation dictionaries passed.')
