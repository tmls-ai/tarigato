"""Copy approved repository documentation into static website articles.
Usage: python3 site/scripts/sync-docs.py . (requires markdown-it-py).
"""
from pathlib import Path
import html
import json
import re
import shutil
import sys
from urllib.parse import urlsplit
from markdown_it import MarkdownIt

repo = Path(sys.argv[1]).resolve()
site = Path(__file__).resolve().parents[1] / 'dist'
(site / 'content').mkdir(exist_ok=True)
(site / 'downloads').mkdir(exist_ok=True)
renderer = MarkdownIt('commonmark', {'html': False}).enable('table')

# These diagrams mirror the repository's Mermaid diagrams. HTML keeps them
# readable on small screens without loading a client-side diagram library.
def blocks(labels):
    return '<div class="doc-flow">' + ''.join('<div class="doc-node">'+html.escape(x)+'</div>' for x in labels) + '</div>'

def diagram(source, locale):
    copy = json.loads((site / 'locales' / f'{locale}.json').read_text())
    if 'CLI[' in source:
        return '<figure class="doc-diagram">'+blocks(['CLI: task, agents, deadline','Go controller'])+'<div class="doc-fan">'+''.join('<div class="doc-node">'+x+'</div>' for x in ['Builder CLI process','Challenger CLI process','Go test process'])+'</div>'+blocks(['Separate Git workspaces','Local patches, result, report'])+'<figcaption>The controller starts each process and writes the artifacts.</figcaption></figure>'
    if 'Baseline{' in source:
        rows=[('Baseline passes?', 'Yes: builder changes source', 'No: stop'),('Original checks pass?', 'Yes: challenger session', 'No: stop'),('One valid test?', 'Yes: run twice on candidate', 'No finding: final checks; invalid: stop'),('Same observed outcome?', 'Passes: final checks', 'Inconsistent: stop'),('Repeated assertion failure?', 'Original suite passes: one repair', 'Original suite fails: stop')]
        return '<figure class="doc-diagram"><div class="doc-node">Clean committed Go repository</div><div class="decision-rows">'+''.join('<div class="decision-row"><strong>'+a+'</strong><span>'+b+'</span><span>'+c+'</span></div>' for a,b,c in rows)+'</div>'+blocks(['Final checks','Save evidence for human review'])+'<figcaption>Stopped runs save the evidence available. Final checks must pass for ready_for_review.</figcaption></figure>'
    names=['baselineBlock','builderBlock','candidateBlock','challengerBlock','verifyBlock','repairBlock','finalBlock','reportBlock']
    branch='<div class="doc-branches"><div>'+html.escape(copy['branchPass'])+' → '+html.escape(copy['finalBlock'])+'</div><div>'+html.escape(copy['branchFail'])+' → '+html.escape(copy['repairBlock'])+'</div><div>'+html.escape(copy['branchStop'])+'</div></div>'
    return '<figure class="doc-diagram">'+blocks([copy[k] for k in names[:5]])+branch+blocks([copy[k] for k in names[6:]])+'<figcaption>'+html.escape(copy['branchNote'])+'</figcaption></figure>'

articles={
 'quickstart.en':'README.md','quickstart.ja':'README.ja.md',
 'quickstart.zh-CN':'README.zh-CN.md','quickstart.de':'README.de.md',
 'design.en':'docs/design.md','example.en':'docs/demo.md',
 'security.en':'SECURITY.md','contributing.en':'CONTRIBUTING.md'
}
article_map={path:key.split('.')[0] for key,path in articles.items()}

def rewrite_url(url, source_file):
    if urlsplit(url).scheme or url.startswith('#'): return url
    parts=urlsplit(url)
    path=(repo/source_file).parent.joinpath(parts.path).resolve()
    try: relative=str(path.relative_to(repo))
    except ValueError: return url
    suffix=('#'+parts.fragment) if parts.fragment else ''
    if relative in article_map:
        locale=next((x for x in ['ja','zh-CN','de'] if relative==f'README.{x}.md'),None)
        return 'docs.html?article='+article_map[relative]+('&lang='+locale if locale else '')+suffix
    if relative.startswith('docs/assets/'):
        destination=site/'assets'/path.name
        shutil.copy2(path,destination)
        return 'assets/'+path.name+suffix
    return 'https://github.com/tmls-ai/tarigato/blob/main/'+relative+suffix

for key, filename in articles.items():
    text=(repo/filename).read_text()
    locale=key.split('.',1)[1]
    if key.startswith('quickstart'):
        # The site's language selector replaces the README language navigation.
        text=re.sub(r'^# Tarigato\n+[^\n]+\n+', '', text)
        text=re.sub(r'^\[!\[.*?\n\n\*[^\n]+\*\n+', '', text, flags=re.M)
        shutil.copy2(repo/filename,site/'downloads'/filename)
    tokens=renderer.parse(text)
    headings=0
    for i,token in enumerate(tokens):
        if token.type=='heading_open':
            label=tokens[i+1].content
            anchor=re.sub(r'[^\w\s-]','',label.lower()).replace(' ','-')
            if token.tag=='h2' and key.startswith('quickstart'):
                anchor=['install','usage','how-it-works','example','results','limits'][headings];headings+=1
            token.attrSet('id',anchor)
        if token.type=='inline':
            for child in token.children or []:
                for attr in ['href','src']:
                    value=child.attrGet(attr)
                    if value:child.attrSet(attr,rewrite_url(value,filename))
        if token.type=='fence' and token.info.strip()=='mermaid':
            token.type='html_block';token.content=diagram(token.content,locale)
    (site/'content'/f'{key}.html').write_text(renderer.renderer.render(tokens,renderer.options,{}).replace('\u2014', ':').replace('\u2013', '-'))
print(f'Synced {len(articles)} articles from {repo.name}.')
