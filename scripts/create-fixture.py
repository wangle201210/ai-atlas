"""Create isolated E2E data; never reads or changes the user's Codex home."""
import json
from pathlib import Path
root=Path(__file__).resolve().parent.parent / '.test-data'
(root/'home/sessions').mkdir(parents=True,exist_ok=True)
for i,(project,title) in enumerate([('/projects/atlas','实现项目用量看板'),('/projects/aurora','优化会话搜索体验')],1):
 session=f'00000000-0000-0000-0000-{i:012d}'
 rows=[
  {'type':'session_meta','timestamp':'2026-09-01T00:00:00Z','payload':{'id':session,'cwd':project,'timestamp':'2026-09-01T00:00:00Z'}},
  {'type':'turn_context','payload':{'turn_id':f'turn-{i}','cwd':project,'model':'fixture-model'}},
  {'type':'response_item','timestamp':'2026-09-01T00:01:00Z','payload':{'type':'message','role':'user','content':[{'type':'input_text','text':title}]}},
  {'type':'response_item','timestamp':'2026-09-01T00:02:00Z','payload':{'type':'message','role':'assistant','content':[{'type':'output_text','text':'已经完成，可以查看测试结果。'}]}},
  {'type':'event_msg','timestamp':'2026-09-01T00:02:00Z','payload':{'type':'token_count','info':{'total_token_usage':{'input_tokens':1000*i,'cached_input_tokens':500*i,'output_tokens':100*i,'reasoning_output_tokens':50*i,'total_tokens':1100*i},'last_token_usage':{'input_tokens':1000*i,'cached_input_tokens':500*i,'output_tokens':100*i,'reasoning_output_tokens':50*i,'total_tokens':1100*i}}}}
 ]
 (root/'home/sessions'/f'{session}.jsonl').write_text(''.join(json.dumps(r,ensure_ascii=False)+'\n' for r in rows))
print(root)
