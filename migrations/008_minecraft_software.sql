ALTER TABLE servers DROP CONSTRAINT servers_server_type_check;
ALTER TABLE servers ADD CONSTRAINT servers_server_type_check CHECK(server_type IN ('VANILLA','PAPER','SPIGOT','PURPUR','FABRIC','FORGE','NEOFORGE'));
ALTER TABLE software_templates DROP CONSTRAINT software_templates_server_type_check;
ALTER TABLE software_templates ADD CONSTRAINT software_templates_server_type_check CHECK(server_type IN ('VANILLA','PAPER','SPIGOT','PURPUR','FABRIC','FORGE','NEOFORGE'));
-- Loader/build pins belong to an approved Minecraft release, not customer input.
ALTER TABLE minecraft_versions ADD COLUMN environment jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(environment)='object');
