import { defineConfig, globalIgnores } from "eslint/config";
import js from "@eslint/js";
import nextVitals from "eslint-config-next/core-web-vitals";
import prettier from "eslint-config-prettier/flat";

export default defineConfig([
  js.configs.recommended,
  ...nextVitals,
  prettier,
  {
    files: ["**/*.{js,mjs}"],
    rules: {
      "no-var": "error",
      "prefer-const": "error",
      eqeqeq: ["error", "always", { null: "ignore" }],
      "react-hooks/exhaustive-deps": "error",
      "@next/next/no-html-link-for-pages": ["error", "./app"],
    },
  },
  globalIgnores([".next/**", "playwright-report/**", "test-results/**"]),
]);
