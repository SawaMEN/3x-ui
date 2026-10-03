import { useMemo, useState } from 'react';
import { Alert, Button, Card, Input, Select, Space, Table, Tag, Typography, message } from 'antd';
import { ReloadOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';

type ApiMsg<T = unknown> = { success?: boolean; msg?: string; obj?: T };
type TrafficPoint = { t: number; up: number; down: number };
type HistoryPayload = { points: TrafficPoint[] };

function formatBytes(value = 0) {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B','KiB','MiB','GiB','TiB']; let size=value, unit=0;
  while(size>=1024&&unit<units.length-1){size/=1024;unit+=1}
  return `${size>=10||unit===0?size.toFixed(0):size.toFixed(1)} ${units[unit]}`;
}

function TrafficChart({points}:{points:TrafficPoint[]}){
  const width=960,height=260,padding=28,max=Math.max(1,...points.flatMap(p=>[p.up,p.down]));
  const x=(i:number)=>padding+(points.length<=1?0:(i*(width-padding*2))/(points.length-1));
  const y=(v:number)=>height-padding-(v/max)*(height-padding*2);
  const path=(k:'up'|'down')=>points.map((p,i)=>`${i===0?'M':'L'} ${x(i)} ${y(p[k])}`).join(' ');
  if(!points.length)return <Alert type="info" showIcon message="No history yet. The first five-minute sample establishes the baseline."/>;
  return <div style={{overflowX:'auto'}}><svg viewBox={`0 0 ${width} ${height}`} style={{width:'100%',minWidth:620,height:280}}><path d={path('up')} fill="none" stroke="#1677ff" strokeWidth="3"/><path d={path('down')} fill="none" stroke="#52c41a" strokeWidth="3"/></svg><Space><Tag color="blue">Upload</Tag><Tag color="green">Download</Tag><Typography.Text type="secondary">Peak {formatBytes(max)} / bucket</Typography.Text></Space></div>;
}

export default function TrafficHistoryPanel(){
  const [messageApi,ctx]=message.useMessage(); const [resource,setResource]=useState<'client'|'inbound'|'outbound'>('client'); const [tag,setTag]=useState(''); const [bucket,setBucket]=useState('1h'); const [points,setPoints]=useState<TrafficPoint[]>([]); const [loading,setLoading]=useState(false);
  const rows=useMemo(()=>points.slice().reverse().map(p=>({key:p.t,time:new Date(p.t*1000).toLocaleString(),up:formatBytes(p.up),down:formatBytes(p.down)})),[points]);
  const load=async()=>{if(!tag.trim()){messageApi.warning('Enter a client email, inbound tag or outbound tag.');return}setLoading(true);try{const msg=await HttpUtil.get(`/panel/api/server/trafficHistory/${resource}/${bucket}`,{tag:tag.trim(),limit:360},{silent:true}) as ApiMsg<HistoryPayload>;if(!msg.success||!msg.obj)throw new Error(msg.msg||'Failed to load traffic history');setPoints(Array.isArray(msg.obj.points)?msg.obj.points:[])}catch(e){messageApi.error(e instanceof Error?e.message:String(e))}finally{setLoading(false)}};
  return <Card>{ctx}<Space direction="vertical" size={16} style={{width:'100%'}}><Space wrap><Select value={resource} style={{width:150}} options={['client','inbound','outbound'].map(v=>({value:v,label:v}))} onChange={setResource}/><Input value={tag} onChange={e=>setTag(e.target.value)} onPressEnter={()=>void load()} placeholder="resource tag / client email" style={{width:300}}/><Select value={bucket} style={{width:120}} options={['5m','15m','30m','1h','6h','12h','1d'].map(v=>({value:v,label:v}))} onChange={setBucket}/><Button type="primary" icon={<ReloadOutlined/>} loading={loading} onClick={()=>void load()}>Load</Button></Space><TrafficChart points={points}/><Table rowKey="key" size="small" dataSource={rows} pagination={{pageSize:12,showSizeChanger:false}} columns={[{title:'Time',dataIndex:'time'},{title:'Upload',dataIndex:'up',width:140},{title:'Download',dataIndex:'down',width:140}]}/></Space></Card>;
}
