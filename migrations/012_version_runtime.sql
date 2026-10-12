-- New Minecraft releases can require a different Java runtime from older releases.
ALTER TABLE minecraft_versions ADD COLUMN docker_image text CHECK(docker_image IS NULL OR length(docker_image) > 0);
