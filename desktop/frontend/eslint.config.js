import js from '@eslint/js';
import tseslint from 'typescript-eslint';
import hooks from 'eslint-plugin-react-hooks';
import refresh from 'eslint-plugin-react-refresh';
export default tseslint.config(
  { ignores: ['node_modules/**', 'test-results/**'] },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  { files: ['**/*.{ts,tsx}'], plugins: { 'react-hooks': hooks, 'react-refresh': refresh }, rules: {
    'react-hooks/rules-of-hooks': 'error', 'react-hooks/exhaustive-deps': 'error',
    'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
  } },
  // Upstream shadcn recipes intentionally colocate these public helper exports.
  { files: ['src/components/ui/button.tsx', 'src/components/ui/sidebar.tsx'], rules: {
    'react-refresh/only-export-components': ['warn', { allowConstantExport: true, allowExportNames: ['buttonVariants', 'useSidebar'] }],
  } },
);
