BEGIN TRANSACTION;
CREATE TABLE `api_tokens` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`token` text NOT NULL,`enabled` numeric DEFAULT true,`created_at` integer,`scope` text NOT NULL DEFAULT "admin",`expires_at` integer NOT NULL DEFAULT 0);
CREATE TABLE `client_external_links` (`id` integer PRIMARY KEY AUTOINCREMENT,`client_id` integer,`kind` text,`value` text,`remark` text,`enable` numeric DEFAULT true,`expiry_time` integer DEFAULT 0,`name_prefix` text,`last_fetch_at` integer DEFAULT 0,`last_fetch_error` text,`sort_index` integer,`created_at` integer);
CREATE TABLE `client_global_traffics` (`id` integer PRIMARY KEY AUTOINCREMENT,`master_guid` text NOT NULL,`email` text NOT NULL,`up` integer,`down` integer,`updated_at` integer);
CREATE TABLE `client_groups` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text NOT NULL,`reset_up` integer DEFAULT 0,`reset_down` integer DEFAULT 0,`created_at` integer,`updated_at` integer);
INSERT INTO "client_groups" VALUES(1,'fixture-group',1024,2048,1793527781191,1793527781191);
CREATE TABLE `client_hwids` (`id` integer PRIMARY KEY AUTOINCREMENT,`sub_id` text NOT NULL,`hwid_hash` text NOT NULL,`first_seen` integer NOT NULL,`last_seen` integer NOT NULL,`user_agent` text,`device_os` text,`os_version` text,`device_model` text);
INSERT INTO "client_hwids" VALUES(1,'fixture-sub-0-0','0000000000000000000000000000000000000000000000000000000000000001',1793491200000,1793491200000,'','fixture','','');
INSERT INTO "client_hwids" VALUES(2,'fixture-sub-0-0','0000000000000000000000000000000000000000000000000000000000000002',1793491200000,1793491200000,'','fixture','','');
INSERT INTO "client_hwids" VALUES(3,'fixture-sub-0-0','0000000000000000000000000000000000000000000000000000000000000003',1793491200000,1793491200000,'','fixture','','');
INSERT INTO "client_hwids" VALUES(4,'fixture-sub-0-0','0000000000000000000000000000000000000000000000000000000000000004',1793491200000,1793491200000,'','fixture','','');
INSERT INTO "client_hwids" VALUES(5,'fixture-sub-0-0','0000000000000000000000000000000000000000000000000000000000000005',1793491200000,1793491200000,'','fixture','','');
CREATE TABLE `client_inbounds` (`client_id` integer,`inbound_id` integer,`flow_override` text,`created_at` integer,PRIMARY KEY (`client_id`,`inbound_id`));
INSERT INTO "client_inbounds" VALUES(1,1,'',1793527781196);
INSERT INTO "client_inbounds" VALUES(2,1,'',1793527781202);
INSERT INTO "client_inbounds" VALUES(3,1,'',1793527781205);
INSERT INTO "client_inbounds" VALUES(4,2,'',1793527781208);
INSERT INTO "client_inbounds" VALUES(5,2,'',1793527781210);
INSERT INTO "client_inbounds" VALUES(6,2,'',1793527781213);
INSERT INTO "client_inbounds" VALUES(7,5,'',1793527781219);
INSERT INTO "client_inbounds" VALUES(8,5,'',1793527781222);
INSERT INTO "client_inbounds" VALUES(9,5,'',1793527781225);
INSERT INTO "client_inbounds" VALUES(10,6,'',1793527781228);
INSERT INTO "client_inbounds" VALUES(11,6,'',1793527781231);
INSERT INTO "client_inbounds" VALUES(12,6,'',1793527781233);
INSERT INTO "client_inbounds" VALUES(13,8,'',1793527781237);
INSERT INTO "client_inbounds" VALUES(14,8,'',1793527781240);
INSERT INTO "client_inbounds" VALUES(15,8,'',1793527781249);
INSERT INTO "client_inbounds" VALUES(16,10,'',1793527781254);
INSERT INTO "client_inbounds" VALUES(17,10,'',1793527781256);
INSERT INTO "client_inbounds" VALUES(18,10,'',1793527781259);
INSERT INTO "client_inbounds" VALUES(19,11,'',1793527781262);
INSERT INTO "client_inbounds" VALUES(20,11,'',1793527781265);
INSERT INTO "client_inbounds" VALUES(21,11,'',1793527781268);
INSERT INTO "client_inbounds" VALUES(22,12,'',1793527781272);
INSERT INTO "client_inbounds" VALUES(23,12,'',1793527781276);
INSERT INTO "client_inbounds" VALUES(24,12,'',1793527781279);
INSERT INTO "client_inbounds" VALUES(25,13,'',1793527781283);
INSERT INTO "client_inbounds" VALUES(26,13,'',1793527781287);
INSERT INTO "client_inbounds" VALUES(27,13,'',1793527781290);
INSERT INTO "client_inbounds" VALUES(28,15,'',1793527781296);
INSERT INTO "client_inbounds" VALUES(29,15,'',1793527781299);
INSERT INTO "client_inbounds" VALUES(30,15,'',1793527781300);
INSERT INTO "client_inbounds" VALUES(31,16,'',1793527781302);
INSERT INTO "client_inbounds" VALUES(32,16,'',1793527781308);
INSERT INTO "client_inbounds" VALUES(33,16,'',1793527781310);
INSERT INTO "client_inbounds" VALUES(34,17,'',1793527781314);
INSERT INTO "client_inbounds" VALUES(35,17,'',1793527781316);
INSERT INTO "client_inbounds" VALUES(36,17,'',1793527781318);
INSERT INTO "client_inbounds" VALUES(37,18,'',1793527781325);
INSERT INTO "client_inbounds" VALUES(38,18,'',1793527781326);
INSERT INTO "client_inbounds" VALUES(39,18,'',1793527781327);
INSERT INTO "client_inbounds" VALUES(40,19,'',1793527781331);
INSERT INTO "client_inbounds" VALUES(41,19,'',1793527781333);
INSERT INTO "client_inbounds" VALUES(42,19,'',1793527781337);
INSERT INTO "client_inbounds" VALUES(43,20,'',1793527781339);
INSERT INTO "client_inbounds" VALUES(44,20,'',1793527781341);
INSERT INTO "client_inbounds" VALUES(45,20,'',1793527781341);
INSERT INTO "client_inbounds" VALUES(46,21,'',1793527781343);
INSERT INTO "client_inbounds" VALUES(47,21,'',1793527781344);
INSERT INTO "client_inbounds" VALUES(48,21,'',1793527781345);
INSERT INTO "client_inbounds" VALUES(49,22,'',1793527781347);
INSERT INTO "client_inbounds" VALUES(50,22,'',1793527781349);
INSERT INTO "client_inbounds" VALUES(51,22,'',1793527781351);
CREATE TABLE `client_traffics` (`id` integer PRIMARY KEY AUTOINCREMENT,`inbound_id` integer,`enable` numeric,`email` text,`up` integer,`down` integer,`expiry_time` integer,`total` integer,`reset` integer DEFAULT 0,`reset_day` integer DEFAULT 0,`reset_max` integer DEFAULT 0,`reset_count` integer DEFAULT 0,`last_online` integer DEFAULT 0,`last_sub_fetch` integer DEFAULT 0,CONSTRAINT `uni_client_traffics_email` UNIQUE (`email`));
INSERT INTO "client_traffics" VALUES(1,1,1,'fixture-vmess-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(2,1,0,'fixture-vmess-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(3,1,1,'fixture-vmess-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(4,2,1,'fixture-vless-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(5,2,0,'fixture-vless-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(6,2,1,'fixture-vless-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(7,5,1,'fixture-trojan-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(8,5,0,'fixture-trojan-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(9,5,1,'fixture-trojan-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(10,6,1,'fixture-shadowsocks-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(11,6,0,'fixture-shadowsocks-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(12,6,1,'fixture-shadowsocks-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(13,8,1,'fixture-wireguard-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(14,8,0,'fixture-wireguard-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(15,8,1,'fixture-wireguard-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(16,10,1,'fixture-hysteria-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(17,10,0,'fixture-hysteria-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(18,10,1,'fixture-hysteria-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(19,11,1,'fixture-mtproto-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(20,11,0,'fixture-mtproto-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(21,11,1,'fixture-mtproto-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(22,12,1,'fixture-amneziawg-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(23,12,0,'fixture-amneziawg-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(24,12,1,'fixture-amneziawg-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(25,13,1,'fixture-tuic-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(26,13,0,'fixture-tuic-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(27,13,1,'fixture-tuic-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(28,15,1,'fixture-trusttunnel-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(29,15,0,'fixture-trusttunnel-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(30,15,1,'fixture-trusttunnel-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(31,16,1,'fixture-naive-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(32,16,0,'fixture-naive-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(33,16,1,'fixture-naive-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(34,17,1,'fixture-anytls-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(35,17,0,'fixture-anytls-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(36,17,1,'fixture-anytls-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(37,18,1,'fixture-shadowtls-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(38,18,0,'fixture-shadowtls-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(39,18,1,'fixture-shadowtls-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(40,19,1,'fixture-mieru-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(41,19,0,'fixture-mieru-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(42,19,1,'fixture-mieru-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(43,20,1,'fixture-vk-turn-proxy-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(44,20,0,'fixture-vk-turn-proxy-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(45,20,1,'fixture-vk-turn-proxy-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(46,21,1,'fixture-sudoku-active@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(47,21,0,'fixture-sudoku-disabled@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(48,21,1,'fixture-sudoku-delayed@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(49,22,1,'fixture-hysteria-active-v1@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(50,22,0,'fixture-hysteria-disabled-v1@example.invalid',12345,67890,4102444800000,10737418240,0,0,0,0,0,0);
INSERT INTO "client_traffics" VALUES(51,22,1,'fixture-hysteria-delayed-v1@example.invalid',12345,67890,-86400000,10737418240,0,0,0,0,0,0);
CREATE TABLE `clients` (`id` integer PRIMARY KEY AUTOINCREMENT,`email` text NOT NULL,`sub_id` text,`hiddify_sub_uri` text DEFAULT "",`uuid` text,`password` text,`auth` text,`flow` text,`security` text,`reverse` text,`wg_private_key` text,`wg_public_key` text,`wg_allowed_ips` text,`wg_pre_shared_key` text,`wg_keep_alive` integer DEFAULT 0,`wg_forwarded_ports` text,`secret` text,`ad_tag` text DEFAULT "",`limit_ip` integer,`limit_hwid` integer DEFAULT 0,`total_gb` integer,`expiry_time` integer,`enable` numeric DEFAULT true,`auto_disabled_by_core` text DEFAULT "",`tg_id` integer,`group_name` text DEFAULT "",`comment` text,`reset` integer DEFAULT 0,`reset_day` integer DEFAULT 0,`reset_max` integer DEFAULT 0,`traffic_reset` text DEFAULT "never",`traffic_reset_day` integer DEFAULT 1,`created_at` integer,`updated_at` integer,`sync_orphaned_at` integer DEFAULT 0);
INSERT INTO "clients" VALUES(1,'fixture-vmess-active@example.invalid','fixture-sub-0-0','','00000000-0000-4000-8000-000000000001','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781194,1793527781196,0);
INSERT INTO "clients" VALUES(2,'fixture-vmess-disabled@example.invalid','fixture-sub-0-1','','00000000-0000-4000-8000-000000000002','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781201,1793527781202,0);
INSERT INTO "clients" VALUES(3,'fixture-vmess-delayed@example.invalid','fixture-sub-0-2','','00000000-0000-4000-8000-000000000003','','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781203,1793527781204,0);
INSERT INTO "clients" VALUES(4,'fixture-vless-active@example.invalid','fixture-sub-1-0','','00000000-0000-4000-8000-000000000004','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781207,1793527781208,0);
INSERT INTO "clients" VALUES(5,'fixture-vless-disabled@example.invalid','fixture-sub-1-1','','00000000-0000-4000-8000-000000000005','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781209,1793527781210,0);
INSERT INTO "clients" VALUES(6,'fixture-vless-delayed@example.invalid','fixture-sub-1-2','','00000000-0000-4000-8000-000000000006','','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781211,1793527781212,0);
INSERT INTO "clients" VALUES(7,'fixture-trojan-active@example.invalid','fixture-sub-4-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781218,1793527781219,0);
INSERT INTO "clients" VALUES(8,'fixture-trojan-disabled@example.invalid','fixture-sub-4-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781221,1793527781221,0);
INSERT INTO "clients" VALUES(9,'fixture-trojan-delayed@example.invalid','fixture-sub-4-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781223,1793527781224,0);
INSERT INTO "clients" VALUES(10,'fixture-shadowsocks-active@example.invalid','fixture-sub-5-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781227,1793527781228,0);
INSERT INTO "clients" VALUES(11,'fixture-shadowsocks-disabled@example.invalid','fixture-sub-5-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781230,1793527781230,0);
INSERT INTO "clients" VALUES(12,'fixture-shadowsocks-delayed@example.invalid','fixture-sub-5-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781232,1793527781233,0);
INSERT INTO "clients" VALUES(13,'fixture-wireguard-active@example.invalid','fixture-sub-7-0','','','','','','','','EIL+m+lSMMKLHfhsQM98jeDNbg0QGHLZqUfqU7+Dr0M=','9wEP+G3+uvQAYSOP93KLBzhC8XWg/pHC+lrpuTVQVH4=','10.0.0.2/32','',25,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781237,1793527781237,0);
INSERT INTO "clients" VALUES(14,'fixture-wireguard-disabled@example.invalid','fixture-sub-7-1','','','','','','','','aG2XaYbTBxlLqjPSu+iohgnj1eokV1OCJgFQxEU+5lc=','3CONB5a6NCz3dO5HYDQD1ahv90B6BHv+QTtNAZhD038=','10.0.0.3/32','',25,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781239,1793527781239,0);
INSERT INTO "clients" VALUES(15,'fixture-wireguard-delayed@example.invalid','fixture-sub-7-2','','','','','','','','cHyhtKTx2B15Vk6iwC3xOxVrky8rtU/xcCHbouI5lFY=','vxjTv4MSzMobV6CpnBaIW6NQygNnpjnXqkvAL9j0oSg=','10.0.0.4/32','',25,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781242,1793527781245,0);
INSERT INTO "clients" VALUES(16,'fixture-hysteria-active@example.invalid','fixture-sub-9-0','','','','synthetic-auth-not-for-production','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781254,1793527781254,0);
INSERT INTO "clients" VALUES(17,'fixture-hysteria-disabled@example.invalid','fixture-sub-9-1','','','','synthetic-auth-not-for-production','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781255,1793527781256,0);
INSERT INTO "clients" VALUES(18,'fixture-hysteria-delayed@example.invalid','fixture-sub-9-2','','','','synthetic-auth-not-for-production','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781258,1793527781259,0);
INSERT INTO "clients" VALUES(19,'fixture-mtproto-active@example.invalid','fixture-sub-10-0','','','','','','','','','','','',0,'','eeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa636f7665722e6578616d706c652e696e76616c6964','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781262,1793527781262,0);
INSERT INTO "clients" VALUES(20,'fixture-mtproto-disabled@example.invalid','fixture-sub-10-1','','','','','','','','','','','',0,'','eeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa636f7665722e6578616d706c652e696e76616c6964','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781263,1793527781264,0);
INSERT INTO "clients" VALUES(21,'fixture-mtproto-delayed@example.invalid','fixture-sub-10-2','','','','','','','','','','','',0,'','eeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa636f7665722e6578616d706c652e696e76616c6964','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781266,1793527781267,0);
INSERT INTO "clients" VALUES(22,'fixture-amneziawg-active@example.invalid','fixture-sub-11-0','','','','','','','','iOxSDy0qdm0G0uf/fvMCPrGfGWnPua4o5vdu0g4icGk=','Yn+ImU9M2cdgKGofR5DE4q+MMY9DtX2gIN2Q9gMaYCw=','10.8.1.2/32','',25,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781270,1793527781271,0);
INSERT INTO "clients" VALUES(23,'fixture-amneziawg-disabled@example.invalid','fixture-sub-11-1','','','','','','','','CExxrIz/4QWhvk8786qeCqobDozFeYcFmfSWyBCVS1k=','R6yERHPbLKkBXVabP+XQSnnQnaCelCEUOTro9P9b/EQ=','10.8.1.3/32','',25,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781273,1793527781276,0);
INSERT INTO "clients" VALUES(24,'fixture-amneziawg-delayed@example.invalid','fixture-sub-11-2','','','','','','','','sHjPghvGok4Vm3X1xiqIEd7SGFKVksuAj/1t1pf4Y3g=','WqV84a5apJA/XCMefoAf5pWX5gxuie2iztKt7eKkllw=','10.8.1.4/32','',25,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781278,1793527781279,0);
INSERT INTO "clients" VALUES(25,'fixture-tuic-active@example.invalid','fixture-sub-12-0','','00000000-0000-4000-8000-000000000037','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781282,1793527781283,0);
INSERT INTO "clients" VALUES(26,'fixture-tuic-disabled@example.invalid','fixture-sub-12-1','','00000000-0000-4000-8000-000000000038','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781285,1793527781286,0);
INSERT INTO "clients" VALUES(27,'fixture-tuic-delayed@example.invalid','fixture-sub-12-2','','00000000-0000-4000-8000-000000000039','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781288,1793527781289,0);
INSERT INTO "clients" VALUES(28,'fixture-trusttunnel-active@example.invalid','fixture-sub-14-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781295,1793527781295,0);
INSERT INTO "clients" VALUES(29,'fixture-trusttunnel-disabled@example.invalid','fixture-sub-14-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781298,1793527781299,0);
INSERT INTO "clients" VALUES(30,'fixture-trusttunnel-delayed@example.invalid','fixture-sub-14-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781300,1793527781300,0);
INSERT INTO "clients" VALUES(31,'fixture-naive-active@example.invalid','fixture-sub-15-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781302,1793527781302,0);
INSERT INTO "clients" VALUES(32,'fixture-naive-disabled@example.invalid','fixture-sub-15-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781303,1793527781308,0);
INSERT INTO "clients" VALUES(33,'fixture-naive-delayed@example.invalid','fixture-sub-15-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781309,1793527781310,0);
INSERT INTO "clients" VALUES(34,'fixture-anytls-active@example.invalid','fixture-sub-16-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781314,1793527781314,0);
INSERT INTO "clients" VALUES(35,'fixture-anytls-disabled@example.invalid','fixture-sub-16-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781315,1793527781316,0);
INSERT INTO "clients" VALUES(36,'fixture-anytls-delayed@example.invalid','fixture-sub-16-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781317,1793527781318,0);
INSERT INTO "clients" VALUES(37,'fixture-shadowtls-active@example.invalid','fixture-sub-17-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781321,1793527781325,0);
INSERT INTO "clients" VALUES(38,'fixture-shadowtls-disabled@example.invalid','fixture-sub-17-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781326,1793527781326,0);
INSERT INTO "clients" VALUES(39,'fixture-shadowtls-delayed@example.invalid','fixture-sub-17-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781327,1793527781327,0);
INSERT INTO "clients" VALUES(40,'fixture-mieru-active@example.invalid','fixture-sub-18-0','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781329,1793527781331,0);
INSERT INTO "clients" VALUES(41,'fixture-mieru-disabled@example.invalid','fixture-sub-18-1','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781333,1793527781333,0);
INSERT INTO "clients" VALUES(42,'fixture-mieru-delayed@example.invalid','fixture-sub-18-2','','','synthetic-password-not-for-production','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781335,1793527781337,0);
INSERT INTO "clients" VALUES(43,'fixture-vk-turn-proxy-active@example.invalid','fixture-sub-19-0','','00000000-0000-4000-8000-000000000058','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781339,1793527781339,0);
INSERT INTO "clients" VALUES(44,'fixture-vk-turn-proxy-disabled@example.invalid','fixture-sub-19-1','','00000000-0000-4000-8000-000000000059','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781340,1793527781340,0);
INSERT INTO "clients" VALUES(45,'fixture-vk-turn-proxy-delayed@example.invalid','fixture-sub-19-2','','00000000-0000-4000-8000-000000000060','','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781341,1793527781341,0);
INSERT INTO "clients" VALUES(46,'fixture-sudoku-active@example.invalid','fixture-sub-20-0','','','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781343,1793527781343,0);
INSERT INTO "clients" VALUES(47,'fixture-sudoku-disabled@example.invalid','fixture-sub-20-1','','','','','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781343,1793527781344,0);
INSERT INTO "clients" VALUES(48,'fixture-sudoku-delayed@example.invalid','fixture-sub-20-2','','','','','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781345,1793527781345,0);
INSERT INTO "clients" VALUES(49,'fixture-hysteria-active-v1@example.invalid','fixture-sub-9-0-v1','','','','synthetic-auth-not-for-production','','','','','','','',0,'','','',2,5,10737418240,4102444800000,1,'',0,'fixture-group','synthetic active',0,0,0,'never',1,1793527781347,1793527781347,0);
INSERT INTO "clients" VALUES(50,'fixture-hysteria-disabled-v1@example.invalid','fixture-sub-9-1-v1','','','','synthetic-auth-not-for-production','','','','','','','',0,'','','',2,5,10737418240,4102444800000,0,'',0,'fixture-group','synthetic disabled',0,0,0,'never',1,1793527781348,1793527781348,0);
INSERT INTO "clients" VALUES(51,'fixture-hysteria-delayed-v1@example.invalid','fixture-sub-9-2-v1','','','','synthetic-auth-not-for-production','','','','','','','',0,'','','',2,5,10737418240,-86400000,1,'',0,'fixture-group','synthetic delayed',0,0,0,'never',1,1793527781350,1793527781350,0);
CREATE TABLE `history_of_seeders` (`id` integer PRIMARY KEY AUTOINCREMENT,`seeder_name` text);
INSERT INTO "history_of_seeders" VALUES(1,'UserPasswordHash');
INSERT INTO "history_of_seeders" VALUES(2,'ClientsTable');
INSERT INTO "history_of_seeders" VALUES(3,'InboundClientsArrayFix');
INSERT INTO "history_of_seeders" VALUES(4,'InboundClientTgIdFix2');
INSERT INTO "history_of_seeders" VALUES(5,'InboundClientSubIdFix');
INSERT INTO "history_of_seeders" VALUES(6,'FreedomFinalRulesReverseFix');
INSERT INTO "history_of_seeders" VALUES(7,'FreedomFinalRulesPrivateEgressBlock');
INSERT INTO "history_of_seeders" VALUES(8,'UppercaseFreedomFinalRulesFix');
INSERT INTO "history_of_seeders" VALUES(9,'InboundRealityFinalmaskTcpStrip');
INSERT INTO "history_of_seeders" VALUES(10,'ApiTokensHash');
INSERT INTO "history_of_seeders" VALUES(11,'LegacyProxySettingsCleanup');
INSERT INTO "history_of_seeders" VALUES(12,'OutboundRemovedKeysFix');
INSERT INTO "history_of_seeders" VALUES(13,'FreedomDomainStrategyFix');
INSERT INTO "history_of_seeders" VALUES(14,'DNSOutboundLegacyKeysFix');
INSERT INTO "history_of_seeders" VALUES(15,'DNSOutboundQTypeZeroFix');
INSERT INTO "history_of_seeders" VALUES(16,'WireguardPeersToClients');
INSERT INTO "history_of_seeders" VALUES(17,'MtprotoSecretsToClients');
INSERT INTO "history_of_seeders" VALUES(18,'NodeInboundsAdopted');
INSERT INTO "history_of_seeders" VALUES(19,'ResetIpLimitNoFail2ban');
INSERT INTO "history_of_seeders" VALUES(20,'VKTurnAcceptClientKeysDefaultOn');
INSERT INTO "history_of_seeders" VALUES(21,'ApiTokensTable');
CREATE TABLE `hosts` (`id` integer PRIMARY KEY AUTOINCREMENT,`group_id` text,`inbound_id` integer NOT NULL,`sort_order` integer DEFAULT 0,`remark` text,`server_description` text,`is_disabled` numeric DEFAULT false,`is_hidden` numeric DEFAULT false,`tags` text,`address` text,`port` integer DEFAULT 0,`security` text DEFAULT "same",`sni` text,`host_header` text,`path` text,`alpn` text,`fingerprint` text,`cipher_suites` text,`override_sni_from_address` numeric,`keep_sni_blank` numeric,`pinned_peer_cert_sha256` text,`verify_peer_cert_by_name` text,`allow_insecure` numeric,`ech_config_list` text,`mux_params` text,`sockopt_params` text,`final_mask` text,`vless_route` text,`exclude_from_sub_types` text,`mihomo_ip_version` text,`mihomo_x25519` numeric,`shuffle_host` numeric,`node_guids` text,`created_at` integer,`updated_at` integer);
INSERT INTO "hosts" VALUES(1,'fixture-host-0',1,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24000,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781206,1793527781206);
INSERT INTO "hosts" VALUES(2,'fixture-host-1',2,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24001,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781214,1793527781214);
INSERT INTO "hosts" VALUES(3,'fixture-host-2',3,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24002,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781216,1793527781216);
INSERT INTO "hosts" VALUES(4,'fixture-host-3',4,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24003,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781217,1793527781217);
INSERT INTO "hosts" VALUES(5,'fixture-host-4',5,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24004,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781226,1793527781226);
INSERT INTO "hosts" VALUES(6,'fixture-host-5',6,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24005,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781234,1793527781234);
INSERT INTO "hosts" VALUES(7,'fixture-host-6',7,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24006,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781236,1793527781236);
INSERT INTO "hosts" VALUES(8,'fixture-host-7',8,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24007,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781251,1793527781251);
INSERT INTO "hosts" VALUES(9,'fixture-host-8',9,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24008,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781252,1793527781252);
INSERT INTO "hosts" VALUES(10,'fixture-host-9',10,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24009,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781260,1793527781260);
INSERT INTO "hosts" VALUES(11,'fixture-host-10',11,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24010,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781269,1793527781269);
INSERT INTO "hosts" VALUES(12,'fixture-host-11',12,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24011,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781280,1793527781280);
INSERT INTO "hosts" VALUES(13,'fixture-host-12',13,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24012,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781292,1793527781292);
INSERT INTO "hosts" VALUES(14,'fixture-host-13',14,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24013,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781293,1793527781293);
INSERT INTO "hosts" VALUES(15,'fixture-host-14',15,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24014,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781301,1793527781301);
INSERT INTO "hosts" VALUES(16,'fixture-host-15',16,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24015,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781313,1793527781313);
INSERT INTO "hosts" VALUES(17,'fixture-host-16',17,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24016,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781321,1793527781321);
INSERT INTO "hosts" VALUES(18,'fixture-host-17',18,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24017,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781328,1793527781328);
INSERT INTO "hosts" VALUES(19,'fixture-host-18',19,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24018,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781338,1793527781338);
INSERT INTO "hosts" VALUES(20,'fixture-host-19',20,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24019,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781342,1793527781342);
INSERT INTO "hosts" VALUES(21,'fixture-host-20',21,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24020,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781346,1793527781346);
INSERT INTO "hosts" VALUES(22,'fixture-host-21',22,0,'fixture host','',0,0,NULL,'vpn.example.invalid',24021,'same','','','',NULL,'','',0,0,NULL,'',0,'','','','','',NULL,'',0,0,NULL,1793527781351,1793527781351);
CREATE TABLE `inbound_client_ips` (`id` integer PRIMARY KEY AUTOINCREMENT,`client_email` text,`ips` text,CONSTRAINT `uni_inbound_client_ips_client_email` UNIQUE (`client_email`));
CREATE TABLE `inbound_fallbacks` (`id` integer PRIMARY KEY AUTOINCREMENT,`master_id` integer NOT NULL,`child_id` integer NOT NULL,`name` text,`alpn` text,`path` text,`dest` text,`xver` integer,`sort_order` integer DEFAULT 0);
CREATE TABLE `inbounds` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer,`up` integer,`down` integer,`total` integer,`remark` text,`sub_sort_index` integer DEFAULT 1,`sort_order` integer DEFAULT 0,`enable` numeric,`expiry_time` integer,`traffic_reset` text DEFAULT "never",`traffic_reset_day` integer DEFAULT 1,`last_traffic_reset_time` integer DEFAULT 0,`listen` text,`port` integer,`protocol` text,`settings` text,`stream_settings` text,`tag` text,`sniffing` text,`node_id` integer,`share_addr_strategy` text DEFAULT "node",`share_addr` text,`disable_flow` numeric DEFAULT false,`origin_node_guid` text,CONSTRAINT `uni_inbounds_tag` UNIQUE (`tag`));
INSERT INTO "inbounds" VALUES(1,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24000,'vmess','{
      "clients": [
        {
          "id": "00000000-0000-4000-8000-000000000001",
          "security": "auto",
          "alterId": 0,
          "email": "fixture-vmess-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-0-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "id": "00000000-0000-4000-8000-000000000002",
          "security": "auto",
          "alterId": 0,
          "email": "fixture-vmess-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-0-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "id": "00000000-0000-4000-8000-000000000003",
          "security": "auto",
          "alterId": 0,
          "email": "fixture-vmess-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-0-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-vmess-0','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(2,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24001,'vless','{
      "clients": [
        {
          "id": "00000000-0000-4000-8000-000000000004",
          "email": "fixture-vless-active@example.invalid",
          "flow": "",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-1-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "id": "00000000-0000-4000-8000-000000000005",
          "email": "fixture-vless-disabled@example.invalid",
          "flow": "",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-1-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "id": "00000000-0000-4000-8000-000000000006",
          "email": "fixture-vless-delayed@example.invalid",
          "flow": "",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-1-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ],
      "decryption": "none",
      "encryption": "none",
      "fallbacks": []
    }','{"network":"tcp","security":"none"}','fixture-vless-1','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(3,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24002,'tunnel','{
      "portMap": {},
      "allowedNetwork": "tcp,udp",
      "followRedirect": false
    }','{"network":"tcp","security":"none"}','fixture-tunnel-2','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(4,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24003,'http','{
      "accounts": [
        {
          "user": "fixture-account",
          "pass": "synthetic-account-password"
        }
      ],
      "allowTransparent": false
    }','{"network":"tcp","security":"none"}','fixture-http-3','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(5,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24004,'trojan','{
      "clients": [
        {
          "password": "synthetic-password-not-for-production",
          "email": "fixture-trojan-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-4-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "password": "synthetic-password-not-for-production",
          "email": "fixture-trojan-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-4-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "password": "synthetic-password-not-for-production",
          "email": "fixture-trojan-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-4-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ],
      "fallbacks": []
    }','{"network":"tcp","security":"tls","tlsSettings":{"serverName":"cover.example.invalid","certificates":[{"certificateFile":"fixture-cert.pem","keyFile":"fixture-key.pem"}]}}','fixture-trojan-4','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(6,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24005,'shadowsocks','{
      "method": "2022-blake3-aes-256-gcm",
      "password": "qcfKLQAjoyaV7SAAnxoBzs/A+osL7MVAke3gtfMXC24=",
      "network": "tcp,udp",
      "clients": [
        {
          "method": "",
          "password": "synthetic-password-not-for-production",
          "email": "fixture-shadowsocks-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-5-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "method": "",
          "password": "synthetic-password-not-for-production",
          "email": "fixture-shadowsocks-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-5-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "method": "",
          "password": "synthetic-password-not-for-production",
          "email": "fixture-shadowsocks-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-5-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ],
      "ivCheck": false
    }','{"network":"tcp","security":"none"}','fixture-shadowsocks-5','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(7,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24006,'mixed','{
      "auth": "password",
      "accounts": [
        {
          "user": "fixture-account",
          "pass": "synthetic-account-password"
        }
      ],
      "udp": false,
      "ip": "127.0.0.1"
    }','{"network":"tcp","security":"none"}','fixture-mixed-6','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(8,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24007,'wireguard','{
      "mtu": 1420,
      "secretKey": "GFr3qEzBbf9NamP8+/1lIbGLZGYhI1hb2t/lhYZd6UY=",
      "peers": [],
      "clients": [
        {
          "privateKey": "EIL+m+lSMMKLHfhsQM98jeDNbg0QGHLZqUfqU7+Dr0M=",
          "publicKey": "9wEP+G3+uvQAYSOP93KLBzhC8XWg/pHC+lrpuTVQVH4=",
          "allowedIPs": [
            "10.0.0.2/32"
          ],
          "keepAlive": 25,
          "email": "fixture-wireguard-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-7-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "privateKey": "aG2XaYbTBxlLqjPSu+iohgnj1eokV1OCJgFQxEU+5lc=",
          "publicKey": "3CONB5a6NCz3dO5HYDQD1ahv90B6BHv+QTtNAZhD038=",
          "allowedIPs": [
            "10.0.0.3/32"
          ],
          "keepAlive": 25,
          "email": "fixture-wireguard-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-7-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "privateKey": "cHyhtKTx2B15Vk6iwC3xOxVrky8rtU/xcCHbouI5lFY=",
          "publicKey": "vxjTv4MSzMobV6CpnBaIW6NQygNnpjnXqkvAL9j0oSg=",
          "allowedIPs": [
            "10.0.0.4/32"
          ],
          "keepAlive": 25,
          "email": "fixture-wireguard-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-7-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ],
      "noKernelTun": false,
      "subnetIp": "10.0.0.0",
      "subnetCidr": 24
    }','{"network":"tcp","security":"none"}','fixture-wireguard-7','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(9,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24008,'tun','{
      "name": "xray0",
      "mtu": 1500,
      "gateway": [],
      "dns": [],
      "userLevel": 0,
      "autoSystemRoutingTable": [],
      "autoOutboundsInterface": "auto"
    }','{"network":"tcp","security":"none"}','fixture-tun-8','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(10,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24009,'hysteria','{
      "version": 2,
      "clients": [
        {
          "auth": "synthetic-auth-not-for-production",
          "email": "fixture-hysteria-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-9-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "auth": "synthetic-auth-not-for-production",
          "email": "fixture-hysteria-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-9-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "auth": "synthetic-auth-not-for-production",
          "email": "fixture-hysteria-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-9-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"tls","tlsSettings":{"serverName":"cover.example.invalid","certificates":[{"certificateFile":"fixture-cert.pem","keyFile":"fixture-key.pem"}]}}','fixture-hysteria-9','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(11,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24010,'mtproto','{
      "fakeTlsDomain": "www.cloudflare.com",
      "clients": [
        {
          "secret": "eeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa636f7665722e6578616d706c652e696e76616c6964",
          "email": "fixture-mtproto-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-10-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "secret": "eeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa636f7665722e6578616d706c652e696e76616c6964",
          "email": "fixture-mtproto-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-10-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "secret": "eeaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa636f7665722e6578616d706c652e696e76616c6964",
          "email": "fixture-mtproto-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-10-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-mtproto-10','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(12,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24011,'amneziawg','{
      "server": {
        "privateKey": "MI6DrG4MZfMtZp0qiAQ1aQ6zFtz9E/Og320+0PyQmWU=",
        "publicKey": "wUuU1Xqg96hpIeFWX63gHrihMOsgtYhMNZEf0j28lFE=",
        "subnetIp": "10.8.1.0",
        "subnetCidr": 24,
        "primaryDns": "8.8.8.8",
        "secondaryDns": "8.8.4.4",
        "externalInterface": "",
        "ipv6Enabled": false,
        "ipv6Subnet": "",
        "ipv6ExternalInterface": "",
        "routeThroughXray": false,
        "jc": 3,
        "jmin": 74,
        "jmax": 253,
        "s1": 85,
        "s2": 47,
        "s3": 38,
        "s4": 23,
        "h1": "214771578",
        "h2": "752084881",
        "h3": "1582229435",
        "h4": "1847043511",
        "i1": "<r 155>",
        "i2": "",
        "i3": "",
        "i4": "",
        "i5": "",
        "headerProtectionKey": "I2E2FH5C/Pq5QzDRxp4l1fQ29EXLe//u5EF59onFZOw=",
        "contentPaddingAddition": "12-26",
        "rekeyAfterTime": "105-115",
        "rekeyTimeout": "3-6",
        "rejectAfterTime": "152-196",
        "keepaliveTimeout": "9-15",
        "maxHandshakeAttempts": "15-40",
        "randomTrailers": true,
        "disableCookies": true
      },
      "clients": [
        {
          "privateKey": "iOxSDy0qdm0G0uf/fvMCPrGfGWnPua4o5vdu0g4icGk=",
          "publicKey": "Yn+ImU9M2cdgKGofR5DE4q+MMY9DtX2gIN2Q9gMaYCw=",
          "allowedIPs": [
            "10.8.1.2/32"
          ],
          "keepAlive": 25,
          "forwardedPorts": "",
          "email": "fixture-amneziawg-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-11-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "privateKey": "CExxrIz/4QWhvk8786qeCqobDozFeYcFmfSWyBCVS1k=",
          "publicKey": "R6yERHPbLKkBXVabP+XQSnnQnaCelCEUOTro9P9b/EQ=",
          "allowedIPs": [
            "10.8.1.3/32"
          ],
          "keepAlive": 25,
          "forwardedPorts": "",
          "email": "fixture-amneziawg-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-11-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "privateKey": "sHjPghvGok4Vm3X1xiqIEd7SGFKVksuAj/1t1pf4Y3g=",
          "publicKey": "WqV84a5apJA/XCMefoAf5pWX5gxuie2iztKt7eKkllw=",
          "allowedIPs": [
            "10.8.1.4/32"
          ],
          "keepAlive": 25,
          "forwardedPorts": "",
          "email": "fixture-amneziawg-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-11-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-amneziawg-11','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(13,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24012,'tuic','{
      "server": {
        "certificate": "fixture-cert.pem",
        "private_key": "fixture-key.pem",
        "congestion_control": "bbr",
        "alpn": [
          "h3",
          "spdy/3.1"
        ],
        "udp_relay_mode": "native",
        "zero_rtt_handshake": true,
        "log_level": "info",
        "max_idle_time": 15,
        "authentication_timeout": 3,
        "max_udp_relay_packet_size": 1500,
        "sni": "cover.example.invalid"
      },
      "clients": [
        {
          "uuid": "00000000-0000-4000-8000-000000000037",
          "id": "00000000-0000-4000-8000-000000000037",
          "password": "synthetic-password-not-for-production",
          "email": "fixture-tuic-active@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-12-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "uuid": "00000000-0000-4000-8000-000000000038",
          "id": "00000000-0000-4000-8000-000000000038",
          "password": "synthetic-password-not-for-production",
          "email": "fixture-tuic-disabled@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-12-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "uuid": "00000000-0000-4000-8000-000000000039",
          "id": "00000000-0000-4000-8000-000000000039",
          "password": "synthetic-password-not-for-production",
          "email": "fixture-tuic-delayed@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-12-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"tls","tlsSettings":{"serverName":"cover.example.invalid","certificates":[{"certificateFile":"fixture-cert.pem","keyFile":"fixture-key.pem"}]}}','fixture-tuic-12','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(14,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24013,'pingtunnel','{
      "key": 0,
      "encrypt": "chacha20",
      "encryptKey": "",
      "maxConn": 0,
      "connectTimeout": 1000,
      "forward": "",
      "congestion": "bb",
      "clients": []
    }','{"network":"tcp","security":"none"}','fixture-pingtunnel-13','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(15,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24014,'trusttunnel','{
      "hostname": "cover.example.invalid",
      "certificate": "fixture-cert.pem",
      "privateKey": "fixture-key.pem",
      "clients": [
        {
          "email": "fixture-trusttunnel-active@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-14-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "email": "fixture-trusttunnel-disabled@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-14-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "email": "fixture-trusttunnel-delayed@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-14-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-trusttunnel-14','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(16,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24015,'naive','{
      "network": "tcp",
      "quicCongestionControl": "bbr",
      "shareLinkFormat": "standard",
      "tls": {
        "enabled": true,
        "serverName": "cover.example.invalid",
        "certificatePath": "fixture-cert.pem",
        "keyPath": "fixture-key.pem"
      },
      "clients": [
        {
          "email": "fixture-naive-active@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-15-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "email": "fixture-naive-disabled@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-15-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "email": "fixture-naive-delayed@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-15-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-naive-15','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(17,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24016,'anytls','{
      "paddingScheme": [
        "stop=8",
        "0=30-30",
        "1=100-400",
        "2=400-500,c,500-1000,c,500-1000,c,500-1000,c,500-1000",
        "3=9-9,500-1000",
        "4=500-1000",
        "5=500-1000",
        "6=500-1000",
        "7=500-1000"
      ],
      "tls": {
        "enabled": true,
        "serverName": "cover.example.invalid",
        "certificatePath": "fixture-cert.pem",
        "keyPath": "fixture-key.pem"
      },
      "clients": [
        {
          "email": "fixture-anytls-active@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-16-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "email": "fixture-anytls-disabled@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-16-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "email": "fixture-anytls-delayed@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-16-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-anytls-16','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(18,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24017,'shadowtls','{
      "version": 3,
      "handshake": {
        "server": "cover.example.invalid",
        "serverPort": 443
      },
      "strictMode": false,
      "wildcardSni": "off",
      "clients": [
        {
          "email": "fixture-shadowtls-active@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-17-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "email": "fixture-shadowtls-disabled@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-17-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "email": "fixture-shadowtls-delayed@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-17-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-shadowtls-17','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(19,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24018,'mieru','{
      "tcpPorts": [],
      "udpPorts": [],
      "multiplexing": "MULTIPLEXING_LOW",
      "handshakeMode": "HANDSHAKE_STANDARD",
      "mtu": 1400,
      "loggingLevel": "INFO",
      "userHintIsMandatory": false,
      "clients": [
        {
          "email": "fixture-mieru-active@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-18-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "email": "fixture-mieru-disabled@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-18-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "email": "fixture-mieru-delayed@example.invalid",
          "password": "synthetic-password-not-for-production",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-18-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-mieru-18','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(20,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24019,'vk-turn-proxy','{
      "forward": {
        "type": "host"
      },
      "sessionMode": "",
      "wgMtu": 1420,
      "threads": 1,
      "useUdp": false,
      "noObfuscation": false,
      "wrapMode": "off",
      "wrapAcceptClientKeys": true,
      "clients": [
        {
          "id": "00000000-0000-4000-8000-000000000058",
          "email": "fixture-vk-turn-proxy-active@example.invalid",
          "enable": true,
          "comment": "synthetic active",
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "limitIp": 2,
          "subId": "fixture-sub-19-0"
        },
        {
          "id": "00000000-0000-4000-8000-000000000059",
          "email": "fixture-vk-turn-proxy-disabled@example.invalid",
          "enable": false,
          "comment": "synthetic disabled",
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "limitIp": 2,
          "subId": "fixture-sub-19-1"
        },
        {
          "id": "00000000-0000-4000-8000-000000000060",
          "email": "fixture-vk-turn-proxy-delayed@example.invalid",
          "enable": true,
          "comment": "synthetic delayed",
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "limitIp": 2,
          "subId": "fixture-sub-19-2"
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-vk-turn-proxy-19','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(21,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24020,'sudoku','{
      "fallbackAddress": "",
      "key": "",
      "aead": "chacha20-poly1305",
      "suspiciousAction": "silent",
      "paddingMin": 5,
      "paddingMax": 15,
      "ascii": "prefer_entropy",
      "customTable": "",
      "customTables": [],
      "enablePureDownlink": true,
      "multiplex": "off",
      "httpmask": {
        "disable": false,
        "mode": "auto",
        "tls": false,
        "host": "",
        "pathRoot": ""
      },
      "clients": [
        {
          "email": "fixture-sudoku-active@example.invalid",
          "sudokuPrivateKey": "",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-20-0",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "email": "fixture-sudoku-disabled@example.invalid",
          "sudokuPrivateKey": "",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-20-1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "email": "fixture-sudoku-delayed@example.invalid",
          "sudokuPrivateKey": "",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-20-2",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"none"}','fixture-sudoku-20','{"enabled":false}',NULL,'node','',0,'');
INSERT INTO "inbounds" VALUES(22,1,12345,67890,10737418240,'schema-validated storage fixture',1,0,1,0,'never',1,0,'',24021,'hysteria','{
      "version": 1,
      "clients": [
        {
          "auth": "synthetic-auth-not-for-production",
          "email": "fixture-hysteria-active-v1@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-9-0-v1",
          "comment": "synthetic active",
          "reset": 0
        },
        {
          "auth": "synthetic-auth-not-for-production",
          "email": "fixture-hysteria-disabled-v1@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": 4102444800000,
          "enable": false,
          "tgId": 0,
          "subId": "fixture-sub-9-1-v1",
          "comment": "synthetic disabled",
          "reset": 0
        },
        {
          "auth": "synthetic-auth-not-for-production",
          "email": "fixture-hysteria-delayed-v1@example.invalid",
          "limitIp": 2,
          "totalGB": 10737418240,
          "expiryTime": -86400000,
          "enable": true,
          "tgId": 0,
          "subId": "fixture-sub-9-2-v1",
          "comment": "synthetic delayed",
          "reset": 0
        }
      ]
    }','{"network":"tcp","security":"tls","tlsSettings":{"serverName":"cover.example.invalid","certificates":[{"certificateFile":"fixture-cert.pem","keyFile":"fixture-key.pem"}]}}','fixture-hysteria-21','{"enabled":false}',NULL,'node','',0,'');
CREATE TABLE `node_client_ips` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_guid` text NOT NULL,`email` text NOT NULL,`ips` text);
CREATE TABLE `node_client_traffics` (`id` integer PRIMARY KEY AUTOINCREMENT,`node_id` integer NOT NULL,`email` text NOT NULL,`up` integer,`down` integer);
CREATE TABLE `nodes` (`id` integer PRIMARY KEY AUTOINCREMENT,`name` text,`remark` text,`scheme` text,`address` text,`port` integer,`base_path` text,`api_token` text,`enable` numeric DEFAULT true,`allow_private_address` numeric DEFAULT false,`tls_verify_mode` text DEFAULT "verify",`pinned_cert_sha256` text,`inbound_sync_mode` text DEFAULT "all",`sort_order` integer DEFAULT 0,`inbound_tags` text,`outbound_tag` text,`guid` text,`status` text DEFAULT "unknown",`last_heartbeat` integer,`latency_ms` integer,`xray_version` text,`singbox_version` text,`singbox_installed` numeric DEFAULT false,`singbox_known` numeric DEFAULT false,`core_type` text,`running_core` text,`panel_version` text,`cpu_pct` real,`mem_pct` real,`uptime_secs` integer,`net_up` integer,`net_down` integer,`last_error` text,`xray_state` text,`xray_error` text,`singbox_state` text,`singbox_error` text,`config_dirty` numeric DEFAULT false,`config_dirty_at` integer,`inbounds_adopted_at` integer DEFAULT 0,`created_at` integer,`updated_at` integer);
INSERT INTO "nodes" VALUES(1,'fixture-node','','https','node.example.invalid',443,'/fixture/','SYNTHETIC-TOKEN-NOT-FOR-PRODUCTION',1,0,'verify','','all',0,NULL,'','fixture-node-guid','unknown',0,0,'','',0,0,'','','',0.0,0.0,0,0,0,'','','','','',0,0,0,1793527781192,1793527781192);
CREATE TABLE `outbound_subscriptions` (`id` integer PRIMARY KEY AUTOINCREMENT,`remark` text,`url` text,`enabled` numeric DEFAULT true,`allow_private` numeric DEFAULT false,`allow_insecure` numeric DEFAULT false,`user_agent` text,`tag_prefix` text,`update_interval` integer DEFAULT 600,`priority` integer DEFAULT 0,`prepend` numeric DEFAULT false,`last_updated` integer,`last_error` text,`last_fetched_outbounds` text,`link_identities` text,`created_at` integer,`updated_at` integer);
CREATE TABLE `outbound_traffics` (`id` integer PRIMARY KEY AUTOINCREMENT,`tag` text,`up` integer DEFAULT 0,`down` integer DEFAULT 0,`total` integer DEFAULT 0,CONSTRAINT `uni_outbound_traffics_tag` UNIQUE (`tag`));
CREATE TABLE `routing_presets` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer NOT NULL,`name` text NOT NULL,`description` text,`rules` text NOT NULL,`created_at` datetime,`updated_at` datetime);
INSERT INTO "routing_presets" VALUES(1,1,'fixture-routing','','[{"type":"field","domain":["domain:example.invalid"],"outboundTag":"direct"}]','2026-11-01 00:00:00+00:00','2026-11-01 00:00:00+00:00');
CREATE TABLE `settings` (`id` integer PRIMARY KEY AUTOINCREMENT,`key` text,`value` text);
INSERT INTO "settings" VALUES(1,'subPath','/zw4kyc4xg7txjjv8/');
INSERT INTO "settings" VALUES(2,'subJsonPath','/agwmoj5vw51tt6fs/');
INSERT INTO "settings" VALUES(3,'subClashPath','/jza1hne7aj39mru6/');
CREATE TABLE `sub_balancers` (`id` integer PRIMARY KEY AUTOINCREMENT,`remark` text,`strategy` text,`inbound_ids` text,`member_weights` text,`sort_order` integer,`enabled` numeric,`created_at` integer,`updated_at` integer);
CREATE TABLE `telemt_ip_geos` (`id` integer PRIMARY KEY AUTOINCREMENT,`ip` text NOT NULL,`city` text,`region` text,`country` text,`country_code` text,`latitude` real,`longitude` real,`last_checked_at` integer NOT NULL);
CREATE TABLE `telemt_ip_histories` (`id` integer PRIMARY KEY AUTOINCREMENT,`ip` text NOT NULL,`users` text,`first_seen_at` integer NOT NULL,`last_seen_at` integer NOT NULL,`download_bytes` integer NOT NULL DEFAULT 0,`upload_bytes` integer NOT NULL DEFAULT 0,`total_bytes` integer NOT NULL DEFAULT 0);
CREATE TABLE `telemt_user_histories` (`id` integer PRIMARY KEY AUTOINCREMENT,`username` text NOT NULL,`active_ips` text,`recent_ips` text,`current_connections` integer NOT NULL DEFAULT 0,`first_seen_at` integer NOT NULL,`last_seen_at` integer NOT NULL,`total_bytes` integer NOT NULL DEFAULT 0,`source_total_bytes` integer NOT NULL DEFAULT 0);
CREATE TABLE `user_sessions` (`id` integer PRIMARY KEY AUTOINCREMENT,`user_id` integer NOT NULL,`session_id_hash` text NOT NULL,`ip_address` text,`user_agent` text,`created_at` datetime,`last_seen_at` datetime,`revoked_at` datetime);
INSERT INTO "user_sessions" VALUES(1,1,'SYNTHETIC_SESSION_HASH','192.0.2.1','auditfixture','2026-11-01 00:00:00+00:00','2026-11-01 00:00:00+00:00',NULL);
CREATE TABLE `users` (`id` integer PRIMARY KEY AUTOINCREMENT,`username` text,`password` text,`login_epoch` integer DEFAULT 0);
INSERT INTO "users" VALUES(1,'fixture-admin','$2a$04$MupCDXERrpPEtkm5TcMFUe3.cOft9m40pLZHLWAE4iUGkCOdaO8kO',0);
CREATE INDEX `idx_inbounds_origin_node_guid` ON `inbounds`(`origin_node_guid`);
CREATE INDEX `idx_inbounds_node_id` ON `inbounds`(`node_id`);
CREATE INDEX `idx_enable_traffic_reset` ON `inbounds`(`enable`,`traffic_reset`);
CREATE INDEX `idx_inbounds_sort_order` ON `inbounds`(`sort_order`);
CREATE INDEX `idx_settings_key` ON `settings`(`key`);
CREATE INDEX `idx_client_traffics_last_online` ON `client_traffics`(`last_online`);
CREATE INDEX `idx_client_traffics_renew` ON `client_traffics`(`expiry_time`,`reset`);
CREATE INDEX `idx_client_traffics_inbound` ON `client_traffics`(`inbound_id`);
CREATE INDEX `idx_nodes_guid` ON `nodes`(`guid`);
CREATE INDEX `idx_nodes_sort_order` ON `nodes`(`sort_order`);
CREATE UNIQUE INDEX `idx_nodes_name` ON `nodes`(`name`);
CREATE UNIQUE INDEX `idx_api_tokens_name` ON `api_tokens`(`name`);
CREATE INDEX `idx_routing_presets_user_id` ON `routing_presets`(`user_id`);
CREATE UNIQUE INDEX `idx_user_sessions_session_id_hash` ON `user_sessions`(`session_id_hash`);
CREATE INDEX `idx_user_sessions_user_id` ON `user_sessions`(`user_id`);
CREATE INDEX `idx_clients_traffic_reset` ON `clients`(`traffic_reset`);
CREATE INDEX `idx_client_record_group` ON `clients`(`group_name`);
CREATE INDEX `idx_clients_tg_id` ON `clients`(`tg_id`);
CREATE INDEX `idx_clients_sub_id` ON `clients`(`sub_id`);
CREATE UNIQUE INDEX `idx_clients_email` ON `clients`(`email`);
CREATE INDEX `idx_client_inbounds_inbound_id` ON `client_inbounds`(`inbound_id`);
CREATE INDEX `idx_client_inbounds_client_id` ON `client_inbounds`(`client_id`);
CREATE INDEX `idx_client_hwids_last_seen` ON `client_hwids`(`last_seen`);
CREATE UNIQUE INDEX `idx_client_hwids_sub_hash` ON `client_hwids`(`sub_id`,`hwid_hash`);
CREATE INDEX `idx_client_hwids_sub_id` ON `client_hwids`(`sub_id`);
CREATE INDEX `idx_client_external_links_client_id` ON `client_external_links`(`client_id`);
CREATE UNIQUE INDEX `idx_client_groups_name` ON `client_groups`(`name`);
CREATE INDEX `idx_inbound_fallbacks_child_id` ON `inbound_fallbacks`(`child_id`);
CREATE INDEX `idx_inbound_fallbacks_master_id` ON `inbound_fallbacks`(`master_id`);
CREATE INDEX `idx_hosts_inbound_id` ON `hosts`(`inbound_id`);
CREATE INDEX `idx_hosts_group_id` ON `hosts`(`group_id`);
CREATE UNIQUE INDEX `idx_node_email` ON `node_client_traffics`(`node_id`,`email`);
CREATE UNIQUE INDEX `idx_nodeip_guid_email` ON `node_client_ips`(`node_guid`,`email`);
CREATE INDEX `idx_client_global_email` ON `client_global_traffics`(`email`);
CREATE UNIQUE INDEX `idx_master_email` ON `client_global_traffics`(`master_guid`,`email`);
CREATE INDEX `idx_telemt_ip_histories_last_seen_at` ON `telemt_ip_histories`(`last_seen_at`);
CREATE INDEX `idx_telemt_ip_histories_first_seen_at` ON `telemt_ip_histories`(`first_seen_at`);
CREATE UNIQUE INDEX `idx_telemt_ip_histories_ip` ON `telemt_ip_histories`(`ip`);
CREATE INDEX `idx_telemt_user_histories_last_seen_at` ON `telemt_user_histories`(`last_seen_at`);
CREATE INDEX `idx_telemt_user_histories_first_seen_at` ON `telemt_user_histories`(`first_seen_at`);
CREATE UNIQUE INDEX `idx_telemt_user_histories_username` ON `telemt_user_histories`(`username`);
CREATE INDEX `idx_telemt_ip_geos_last_checked_at` ON `telemt_ip_geos`(`last_checked_at`);
CREATE UNIQUE INDEX `idx_telemt_ip_geos_ip` ON `telemt_ip_geos`(`ip`);
CREATE INDEX idx_clients_email_lower ON clients (LOWER(email));
DELETE FROM "sqlite_sequence";
INSERT INTO "sqlite_sequence" VALUES('settings',3);
INSERT INTO "sqlite_sequence" VALUES('users',1);
INSERT INTO "sqlite_sequence" VALUES('history_of_seeders',21);
INSERT INTO "sqlite_sequence" VALUES('client_groups',1);
INSERT INTO "sqlite_sequence" VALUES('nodes',1);
INSERT INTO "sqlite_sequence" VALUES('inbounds',22);
INSERT INTO "sqlite_sequence" VALUES('clients',51);
INSERT INTO "sqlite_sequence" VALUES('client_hwids',5);
INSERT INTO "sqlite_sequence" VALUES('client_traffics',51);
INSERT INTO "sqlite_sequence" VALUES('hosts',22);
INSERT INTO "sqlite_sequence" VALUES('routing_presets',1);
INSERT INTO "sqlite_sequence" VALUES('user_sessions',1);
COMMIT;
