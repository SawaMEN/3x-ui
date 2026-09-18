import { useEffect, useState } from 'react';
import { Alert, Button, Card, Col, Form, Input, InputNumber, Row, Space, Switch, Tag, message } from 'antd';
import { ReloadOutlined, PlayCircleOutlined, StopOutlined, SyncOutlined } from '@ant-design/icons';
import { HttpUtil } from '@/utils';

type Status={installed:boolean;active:boolean;enabled:boolean;configured:boolean};
type Config={enabled:boolean;port:number;secret:string;ipv4:boolean;ipv6:boolean;prefer:number;fastMode:boolean;classic:boolean;secure:boolean;tls:boolean;upstreamType:string};

const defaults:Config={enabled:false,port:8443,secret:'',ipv4:true,ipv6:true,prefer:4,fastMode:true,classic:false,secure:false,tls:true,upstreamType:'direct'};

export default function TelemtPage(){
 const [form]=Form.useForm<Config>(); const [status,setStatus]=useState<Status>({installed:false,active:false,enabled:false,configured:false}); const [loading,setLoading]=useState(false);
 const refresh=async()=>{const [s,c]=await Promise.all([HttpUtil.get('/panel/api/telemt/status'),HttpUtil.get('/panel/api/telemt/config')]); if(s?.success)setStatus(s.obj); if(c?.success)form.setFieldsValue({...defaults,...c.obj});};
 useEffect(()=>{void refresh();},[]);
 const save=async(v:Config)=>{setLoading(true);try{const r=await HttpUtil.post('/panel/api/telemt/config',v);if(r?.success){message.success('Конфигурация Telemt сохранена');await refresh();}else message.error(r?.msg||'Не удалось сохранить');}finally{setLoading(false);}};
 const action=async(a:'start'|'stop'|'restart')=>{setLoading(true);try{const r=await HttpUtil.post('/panel/api/telemt/action',{action:a});if(r?.success){message.success('Команда выполнена');await refresh();}else message.error(r?.msg||'Команда не выполнена');}finally{setLoading(false);}};
 return <div className="page-shell"><Row gutter={[16,16]}><Col xs={24}><Card title="Telemt — MTProto" extra={<Button icon={<ReloadOutlined/>} onClick={refresh}>Обновить</Button>}>
 {!status.installed&&<Alert type="warning" showIcon message="Telemt не установлен. Установите последнюю сборку проекта, чтобы получить бинарник и systemd-сервис." style={{marginBottom:16}}/>}
 <Space wrap style={{marginBottom:16}}><Tag color={status.installed?'green':'red'}>Бинарник: {status.installed?'установлен':'нет'}</Tag><Tag color={status.active?'green':'default'}>Сервис: {status.active?'запущен':'остановлен'}</Tag><Tag color={status.enabled?'green':'default'}>Автозапуск: {status.enabled?'да':'нет'}</Tag><Tag>Конфиг: {status.configured?'есть':'нет'}</Tag></Space>
 <Form form={form} layout="vertical" onFinish={save} initialValues={defaults}>
 <Row gutter={16}><Col xs={24} md={8}><Form.Item name="port" label="Порт" rules={[{required:true},{type:'number',min:1,max:65535}]}><InputNumber style={{width:'100%'}}/></Form.Item></Col><Col xs={24} md={16}><Form.Item name="secret" label="Секрет (32 hex)" rules={[{required:true},{pattern:/^[0-9a-fA-F]{32}$/ ,message:'Нужны ровно 32 hex-символа'}]}><Input.Password placeholder="32 hex символа"/></Form.Item></Col></Row>
 <Row gutter={16}><Col span={8}><Form.Item name="ipv4" label="IPv4" valuePropName="checked"><Switch/></Form.Item></Col><Col span={8}><Form.Item name="ipv6" label="IPv6" valuePropName="checked"><Switch/></Form.Item></Col><Col span={8}><Form.Item name="fastMode" label="Fast mode" valuePropName="checked"><Switch/></Form.Item></Col></Row>
 <Row gutter={16}><Col span={8}><Form.Item name="classic" label="Classic" valuePropName="checked"><Switch/></Form.Item></Col><Col span={8}><Form.Item name="secure" label="Secure" valuePropName="checked"><Switch/></Form.Item></Col><Col span={8}><Form.Item name="tls" label="TLS" valuePropName="checked"><Switch/></Form.Item></Col></Row>
 <Form.Item name="prefer" label="Предпочтительный IP"><InputNumber min={4} max={6} addonBefore="IPv"/></Form.Item>
 <Space wrap><Button type="primary" htmlType="submit" loading={loading}>Сохранить</Button><Button icon={<PlayCircleOutlined/>} onClick={()=>action('start')} disabled={!status.installed} loading={loading}>Запустить</Button><Button icon={<StopOutlined/>} onClick={()=>action('stop')} loading={loading}>Остановить</Button><Button icon={<SyncOutlined/>} onClick={()=>action('restart')} disabled={!status.installed} loading={loading}>Перезапустить</Button></Space>
 </Form></Card></Col></Row></div>;
}