#!/usr/bin/env python3
"""Check the rendered deployment boundary without starting containers."""

import json
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parent.parent


class ComposeTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        environment = dict(os.environ)
        environment.update({
            "APP_ENV": "development",
            "POSTGRES_PASSWORD": "compose-test-only",
            "DATABASE_URL": "postgres://hosting:compose-test-only@postgres:5432/hosting?sslmode=disable",
            "PANEL_DATABASE_PASSWORD": "compose-panel-test-only",
            "PANEL_DATABASE_ROOT_PASSWORD": "compose-root-test-only",
            "PTERODACTYL_URL": "http://panel",
            "PTERODACTYL_CLIENT_API_KEY": "ptlc_compose-test-only",
            "PTERODACTYL_APPLICATION_API_KEY": "ptla_compose-test-only",
            "PTERODACTYL_WEBSOCKET_ORIGINS": "ws://wings.localhost:8180",
            "PTERODACTYL_TRANSFER_ORIGINS": "http://wings.localhost:8180",
        })
        command = [
            "docker", "compose", "--env-file", ".env.example",
            "-f", "docker-compose.yml", "-f", "docker-compose.pterodactyl.yml",
            "-f", "docker-compose.wings.yml", "--profile", "frontend",
            "--profile", "pterodactyl", "--profile", "wings",
            "config", "--format", "json",
        ]
        result = subprocess.run(command, cwd=ROOT, env=environment, capture_output=True, check=True)
        cls.services = json.loads(result.stdout)["services"]

    def test_only_wings_has_host_privileges(self):
        for name, service in self.services.items():
            targets = {mount["target"] for mount in service.get("volumes", [])}
            if name != "wings":
                self.assertNotIn("/var/run/docker.sock", targets, name)
                self.assertNotIn("/var/lib/pterodactyl", targets, name)
                self.assertNotEqual(service.get("network_mode"), "host", name)
            self.assertFalse(service.get("privileged", False), name)
        self.assertIn("wings", self.services["wings"]["profiles"])

    def test_host_container_paths_match_for_docker_mounts(self):
        mounts = {mount["target"]: mount for mount in self.services["wings"]["volumes"]}
        for path in ["/var/lib/pterodactyl", "/tmp/pterodactyl", "/var/run/docker.sock"]:
            self.assertEqual(mounts[path]["source"], path)
            # Compose omits false-valued bind options in its normalized output.
            self.assertFalse(mounts[path]["bind"].get("create_host_path", False))

    def test_api_credentials_stay_separate(self):
        backend = self.services["backend"]["environment"]
        worker = self.services["worker"]["environment"]
        self.assertNotIn("PTERODACTYL_APPLICATION_API_KEY", backend)
        self.assertNotIn("PTERODACTYL_CLIENT_API_KEY", worker)
        frontend = self.services["frontend"].get("environment", {})
        self.assertFalse(any("KEY" in key or "SECRET" in key for key in frontend))
        build_args = self.services["frontend"]["build"].get("args", {})
        self.assertEqual(set(build_args), {"NEXT_PUBLIC_API_URL"})

    def test_panel_revision_and_persistent_keys(self):
        panel = self.services["panel"]
        self.assertRegex(panel["build"]["context"], r"panel\.git#[0-9a-f]{40}$")
        self.assertNotIn("latest", panel["image"])
        targets = {mount["target"] for mount in panel["volumes"]}
        self.assertIn("/app/var", targets)
        self.assertEqual(panel["environment"]["DB_CONNECTION"], "mysql")
        self.assertEqual(self.services["backend"]["environment"]["DATABASE_URL"].split(":")[0], "postgres")
        self.assertFalse(self.services["panel-database"].get("ports"))
        self.assertFalse(self.services["panel-cache"].get("ports"))


if __name__ == "__main__":
    unittest.main()
