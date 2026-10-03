import type { Section } from './endpoints';

export const runtimeSections: Section[] = [
  {
    id: 'traffic-history',
    title: 'Traffic history',
    description: 'Durable upload/download history aggregated from the panel traffic counters.',
    endpoints: [
      {
        method: 'GET',
        path: '/panel/api/server/trafficHistory/:resource/:bucket',
        summary: 'Get aggregated upload/download history for a client, inbound, or outbound.',
        params: [
          {
            name: 'resource',
            in: 'path',
            type: 'string',
            enum: ['client', 'inbound', 'outbound'],
            desc: 'Resource type.',
          },
          {
            name: 'bucket',
            in: 'path',
            type: 'string',
            enum: ['5m', '15m', '30m', '1h', '6h', '12h', '1d'],
            desc: 'Aggregation bucket.',
          },
          {
            name: 'tag',
            in: 'query',
            type: 'string',
            desc: 'Client email/name, inbound tag, or outbound tag.',
          },
          {
            name: 'limit',
            in: 'query',
            type: 'integer',
            optional: true,
            defaultValue: 360,
            desc: 'Maximum number of points. The backend caps this at 1000.',
          },
        ],
      },
    ],
  },
];
