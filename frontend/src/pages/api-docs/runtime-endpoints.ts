import type { Section } from './endpoints';

export const runtimeSections: Section[] = [{
  id:'singbox-runtime', title:'Sing-box Runtime', description:'Live sing-box sessions, connection control and durable traffic history.', endpoints:[
    {method:'GET',path:'/panel/api/singbox/connections',summary:'List live sing-box connections.',params:[{name:'resource',in:'query',type:'string',optional:true,enum:['user','client','inbound','outbound'],desc:'Filter namespace.'},{name:'tag',in:'query',type:'string',optional:true,desc:'User, inbound or outbound tag.'}]},
    {method:'POST',path:'/panel/api/singbox/connections/:id/close',summary:'Close one live sing-box connection.',params:[{name:'id',in:'path',type:'string',desc:'Connection ID.'}]},
    {method:'GET',path:'/panel/api/server/trafficHistory/:resource/:bucket',summary:'Get aggregated upload/download history.',params:[{name:'resource',in:'path',type:'string',enum:['client','inbound','outbound'],desc:'Resource type.'},{name:'bucket',in:'path',type:'string',enum:['5m','15m','30m','1h','6h','12h','1d'],desc:'Aggregation bucket.'},{name:'tag',in:'query',type:'string',desc:'Client email/name, inbound or outbound tag.'},{name:'limit',in:'query',type:'integer',optional:true,defaultValue:360,desc:'Maximum points, capped at 1000.'}]},
  ]
}];
