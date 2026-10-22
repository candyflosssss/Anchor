import codex from '../../assets/agent-brands/openai.svg';
import claude from '../../assets/agent-brands/claude.svg';
import gemini from '../../assets/agent-brands/gemini-color.svg';
import opencode from '../../assets/agent-brands/opencode.svg';
import dsh from '../../assets/agent-brands/deepseek-color.svg';

export const agentBrands: Record<string, { name: string; logo: string; monochrome?: boolean }> = {
  codex: { name: 'Codex', logo: codex, monochrome: true },
  claude: { name: 'Claude Code CLI', logo: claude },
  'claude-desktop': { name: 'Claude Desktop', logo: claude },
  gemini: { name: 'Gemini CLI', logo: gemini },
  opencode: { name: 'OpenCode', logo: opencode, monochrome: true },
  dsh: { name: 'DSH Web', logo: dsh },
  'dsh-desktop': { name: 'DSH Desktop', logo: dsh },
};
