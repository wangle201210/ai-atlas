export interface Usage {
  input: number;
  cached: number;
  output: number;
  reasoning: number;
  total: number;
}
export interface Project {
  path: string;
  name: string;
  sessions: number;
  bytes: number;
  usage: Usage;
}
export interface Snapshot {
  projects: Project[];
  usage: Usage;
  sessions: number;
  bytes: number;
  days: { date: string; total: number }[];
  storage: { path: string; bytes: number; error: string }[];
  home: string;
  database: string;
  tempRoots: string[];
  lastScan: string;
}
export interface Session {
  id: string;
  path: string;
  project: string;
  title: string;
  model: string;
  created: string;
  updated: string;
  parent: string;
  size: number;
  mtime: number;
  archived: boolean;
  missing: boolean;
  warning: string;
  usage: Usage;
}
export interface SessionQuery {
  search: string;
  project: string;
  state: string;
  sort: string;
  page: number;
}
export interface SessionPage {
  items: Session[];
  total: number;
}
export interface Detail {
  session: Session;
  messages: { role: string; text: string; time: string }[];
  truncated: boolean;
}
export interface ScanStatus {
  running: boolean;
  phase: string;
  done: number;
  total: number;
  errors: string[];
  finished: string;
}
export interface Evidence {
  sessionId: string;
  project: string;
  kind: string;
  path: string;
}
export interface TempFile {
  path: string;
  name: string;
  bytes: number;
  modified: string;
  mtime: number;
  isDir: boolean;
  link: boolean;
  evidence: Evidence[];
  error: string;
}
export interface CleanPlan {
  token: string;
  kind: string;
  items: {
    id: string;
    path: string;
    bytes: number;
    blocked: string;
    mtime: number;
  }[];
  bytes: number;
  expires: string;
}
export interface CleanResult {
  id: string;
  success: boolean;
  message: string;
}
