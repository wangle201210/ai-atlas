"""Rebuild editable AI Atlas icon candidates and the numbered contact sheet.
Requires rsvg-convert. No external API or image-generation model is used.
"""
from pathlib import Path
import base64, json, subprocess
ROOT = Path(__file__).resolve().parent

def icon(name, colors, art):
    start,end=colors
    svg=f'''<svg xmlns="http://www.w3.org/2000/svg" width="1024" height="1024" viewBox="0 0 1024 1024">
<defs>
<linearGradient id="tile" x1="0" y1="0" x2="1" y2="1"><stop stop-color="{start}"/><stop offset="1" stop-color="{end}"/></linearGradient>
<linearGradient id="white" x1="0" y1="0" x2=".8" y2="1"><stop stop-color="#ffffff"/><stop offset="1" stop-color="#dce7ff"/></linearGradient>
<linearGradient id="mint" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#b9fff0"/><stop offset="1" stop-color="#37d9c4"/></linearGradient>
<linearGradient id="blue" x1="0" y1="0" x2="1" y2="1"><stop stop-color="#86b9ff"/><stop offset="1" stop-color="#5270f5"/></linearGradient>
<filter id="shadow" x="-40%" y="-40%" width="180%" height="190%"><feDropShadow dx="0" dy="20" stdDeviation="19" flood-color="#081d47" flood-opacity=".22"/></filter>
</defs>
<rect x="64" y="64" width="896" height="896" rx="206" fill="url(#tile)"/>
<rect x="65" y="65" width="894" height="894" rx="205" fill="none" stroke="#fff" stroke-opacity=".22" stroke-width="2"/>
{art}
</svg>'''
    (ROOT/f'{name}.svg').write_text(svg)

icon('02-session-vault',('#253958','#101c33'),'''
<g filter="url(#shadow)">
 <rect x="258" y="222" width="508" height="128" rx="43" fill="url(#blue)"/>
 <rect x="230" y="358" width="564" height="140" rx="43" fill="url(#white)"/>
 <path d="M248 502H776Q808 502 808 537V726Q808 778 756 778H268Q216 778 216 726V537Q216 502 248 502Z" fill="url(#white)"/>
 <path d="M454 566H570" stroke="#354f83" stroke-width="25" stroke-linecap="round"/>
 <path d="m616 642 66 49 116-133" fill="none" stroke="url(#mint)" stroke-width="51" stroke-linecap="round" stroke-linejoin="round"/>
</g>''')
icon('03-conversation-orbit',('#8a8bff','#443cc1'),'''
<g filter="url(#shadow)">
 <path d="M327 429Q327 320 442 320H700Q806 320 806 430V600Q806 708 700 708H657L548 788V708H442Q327 708 327 600Z" fill="#a7bcff"/>
 <path d="M202 332Q202 230 310 230H555Q666 230 666 338V497Q666 604 555 604H361L265 676V595Q202 568 202 497Z" fill="url(#white)"/>
 <circle cx="320" cy="420" r="25" fill="#6869db"/><circle cx="430" cy="420" r="25" fill="#6869db"/><circle cx="540" cy="420" r="25" fill="#6869db"/>
 <path d="m674 513 21 56 57 21-57 21-21 56-21-56-57-21 57-21Z" fill="url(#mint)"/>
</g>''')
icon('04-project-compass',('#4f8eff','#2851c8'),'''
<g filter="url(#shadow)">
 <path d="M234 341Q234 298 277 298H420L468 351H744Q786 351 786 396V714Q786 764 736 764H283Q234 764 234 715Z" fill="#8cb5ff"/>
 <path d="M225 405H770Q807 405 800 447L761 731Q756 769 717 769H264Q226 769 223 731L196 447Q193 405 225 405Z" fill="url(#white)"/>
 <circle cx="512" cy="573" r="113" fill="#e0e9ff"/>
 <path d="m583 479-42 129-123 65 47-139Z" fill="#4567d7"/>
 <path d="m583 479-118 55 76 74Z" fill="#76ddcf"/>
</g>''')
icon('05-token-signal',('#123f52','#102938'),'''
<g filter="url(#shadow)">
 <path d="M512 205 773 353V659L512 809 251 659V353Z" fill="none" stroke="#7fe5d8" stroke-width="32" stroke-linejoin="round"/>
 <rect x="348" y="518" width="78" height="159" rx="25" fill="#efffff"/>
 <rect x="473" y="416" width="78" height="261" rx="25" fill="#efffff"/>
 <rect x="598" y="316" width="78" height="361" rx="25" fill="url(#mint)"/>
</g>''')
icon('06-space-lens',('#faf8f0','#e5eaf3'),'''
<g filter="url(#shadow)" fill="none" stroke-width="116">
 <circle cx="512" cy="512" r="211" stroke="#d8e0f3"/>
 <path d="M512 301A211 211 0 1 1 311 577" stroke="#5472e9" stroke-linecap="round"/>
 <path d="M308 458A211 211 0 0 1 442 313" stroke="#41baa9" stroke-linecap="round"/>
</g>
<path d="M512 418 568 513 512 609 456 513Z" fill="#24456f"/>
<circle cx="715" cy="302" r="39" fill="#f7b958"/>''')
icon('07-atlas-monogram',('#162333','#101821'),'''
<g filter="url(#shadow)">
 <path d="M704 317C613 215 376 213 281 356 186 499 254 721 420 761" fill="none" stroke="url(#mint)" stroke-width="83" stroke-linecap="round"/>
 <path d="M382 734 538 288 727 734" fill="none" stroke="url(#white)" stroke-width="77" stroke-linecap="round" stroke-linejoin="round"/>
 <path d="M439 580H660" stroke="url(#white)" stroke-width="65" stroke-linecap="round"/>
</g>''')
icon('08-project-network',('#eef4ff','#dce7fd'),'''
<g fill="none" stroke="#5d7cba" stroke-width="30" stroke-linecap="round" stroke-linejoin="round"><path d="M339 300H553Q688 300 688 435V511"/><path d="M339 300V616Q339 706 436 706H558"/></g>
<g filter="url(#shadow)">
 <rect x="219" y="203" width="229" height="220" rx="64" fill="#466ae7"/>
 <path d="m283 311 39 41 78-85" fill="none" stroke="#fff" stroke-width="25" stroke-linecap="round" stroke-linejoin="round"/>
 <rect x="590" y="477" width="203" height="194" rx="58" fill="#40bba9"/>
 <rect x="512" y="655" width="175" height="169" rx="48" fill="#849cf2"/>
 <circle cx="692" cy="574" r="34" fill="#d5fff7"/>
 <path d="M565 739H634" stroke="#fff" stroke-width="19" stroke-linecap="round"/>
</g>''')
icon('09-terminal-spark',('#ffae80','#df6469'),'''
<g filter="url(#shadow)">
 <rect x="204" y="263" width="616" height="497" rx="96" fill="#263245"/>
 <rect x="233" y="289" width="558" height="65" rx="32" fill="#35435a"/>
 <circle cx="280" cy="321" r="12" fill="#ffb19e"/><circle cx="325" cy="321" r="12" fill="#ffc96b"/><circle cx="370" cy="321" r="12" fill="#87dcc7"/>
 <path d="m326 438 102 93-102 93" fill="none" stroke="#fff5e9" stroke-width="43" stroke-linecap="round" stroke-linejoin="round"/>
 <path d="M503 626H646" stroke="#fff5e9" stroke-width="40" stroke-linecap="round"/>
 <path d="m720 410 24 66 69 24-69 24-24 66-24-66-69-24 69-24Z" fill="#a2ffdf"/>
</g>''')
icon('10-workspace-atlas',('#268b89','#15515e'),'''
<g filter="url(#shadow)">
 <path d="m214 301 203-75 200 73 193-72v495l-193 74-200-75-203 75Z" fill="url(#white)" stroke="url(#white)" stroke-width="14" stroke-linejoin="round"/>
 <path d="m417 226 200 73v497l-200-75Z" fill="#bed6ec"/>
 <path d="m296 601 149-134 118 94 147-187" fill="none" stroke="#3863a0" stroke-width="28" stroke-linecap="round" stroke-linejoin="round"/>
 <circle cx="296" cy="601" r="30" fill="#3863a0"/>
 <circle cx="710" cy="374" r="40" fill="#36cdb5" stroke="#f1fffd" stroke-width="13"/>
</g>''')

entries=[
 ('01-atlas-navigator','Navigator','导航罗盘','C 形轨道与 A 形指针，突出 AI Atlas 的品牌识别。'),
 ('02-session-vault','Session Vault','会话档案库','三层归档盒与薄荷绿勾选，强调会话整理与清理。'),
 ('03-conversation-orbit','Conversation','会话轨道','前后叠放的气泡，代表多会话与上下文。'),
 ('04-project-compass','Project Compass','项目指南针','文件夹与罗盘，直观表达项目目录管理。'),
 ('05-token-signal','Token Signal','用量信号','六边形容器与阶梯柱形图，强调 Token 数据。'),
 ('06-space-lens','Space Lens','空间透镜','环形存储分布与中心指针，适合存储分析定位。'),
 ('07-atlas-monogram','Atlas Monogram','字母徽记','简洁的 C + A 字母组合，小尺寸辨识度高。'),
 ('08-project-network','Project Network','项目脉络','连接的项目节点，表达项目、会话和文件的关联。'),
 ('09-terminal-spark','Terminal Spark','终端灵光','终端命令与亮点，偏开发者工具气质。'),
 ('10-workspace-atlas','Workspace Atlas','工作空间地图','折叠地图和路径，呼应 Atlas 与文件追溯。'),
]
for name,*_ in entries:
 subprocess.run(['rsvg-convert','-w','1024','-h','1024',str(ROOT/f'{name}.svg'),'-o',str(ROOT/f'{name}.png')],check=True)
 subprocess.run(['rsvg-convert','-w','64','-h','64',str(ROOT/f'{name}.svg'),'-o',str(ROOT/f'{name}-64.png')],check=True)

parts=['<svg xmlns="http://www.w3.org/2000/svg" width="1800" height="1000" viewBox="0 0 1800 1000"><rect width="1800" height="1000" fill="#f5f7fc"/><text x="70" y="68" font-family="Arial,sans-serif" font-size="30" font-weight="700" fill="#1b2d47">AI ATLAS</text><text x="70" y="102" font-family="Arial,sans-serif" font-size="15" fill="#687892">10 app icon directions · 1024 px · editable SVG + PNG</text>']
for i,(name,title,zh,desc) in enumerate(entries):
 x=55+(i%5)*342;y=142+(i//5)*412
 b64=base64.b64encode((ROOT/f'{name}.png').read_bytes()).decode()
 parts.append(f'<rect x="{x}" y="{y}" width="322" height="384" rx="20" fill="#fff" stroke="#e2e8f2"/><image x="{x+25}" y="{y+12}" width="272" height="272" href="data:image/png;base64,{b64}"/><text x="{x+28}" y="{y+319}" font-family="Arial,sans-serif" font-weight="700" font-size="18" fill="#203451">{i+1:02d}  {title}</text><image x="{x+28}" y="{y+338}" width="32" height="32" href="data:image/png;base64,{b64}"/><text x="{x+73}" y="{y+360}" font-family="Arial,sans-serif" font-size="12" fill="#7b8ba1">32 px preview</text>')
parts.append('</svg>')
(ROOT/'contact-sheet.svg').write_text(''.join(parts))
subprocess.run(['rsvg-convert',str(ROOT/'contact-sheet.svg'),'-o',str(ROOT/'contact-sheet.png')],check=True)
(ROOT/'manifest.json').write_text(json.dumps([dict(id=i+1,file=name,title=title,name=zh,description=desc) for i,(name,title,zh,desc) in enumerate(entries)],ensure_ascii=False,indent=2)+'\n')
(ROOT/'README.md').write_text('# AI Atlas 图标候选\n\n10 个独立设计，每个提供 1024×1024 PNG、64×64 PNG 和可编辑 SVG。\n\n![候选对比](contact-sheet.png)\n\n'+ '\n'.join(f'- **{i+1:02d} {zh}**：{desc} [PNG]({name}.png) · [SVG]({name}.svg)' for i,(name,title,zh,desc) in enumerate(entries))+'\n\n推荐 01（品牌识别）、02（功能直观）或 07（简洁）。\n\n这些为直接制作的矢量图标，没有使用图片生成模型/API。运行 `python3 design/icons/generate-candidates.py` 可重建 02–10 的矢量文件以及所有 PNG 和对比图（需 rsvg-convert）；01 使用已保存的 SVG 源文件。\n\n选定后需同步替换 `build/appicon.svg`、`build/appicon.png` 和 `build/appicon.icon/` 中的默认 Wails 资源，再重新生成 `Assets.car`、ICNS 和 ICO。已选定 05 Token Signal，并应用到桌面图标和侧边栏。\n')
print('Generated 10 icons, small-size previews, and contact-sheet.png')
