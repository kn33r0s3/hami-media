import { spawn } from 'node:child_process';
import http from 'node:http';
import fs from 'node:fs';

const chromePath = '/Users/nirojpaudyal/Library/Caches/ms-playwright/chromium-1243/chrome-mac-arm64/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing';

// Test runner script that opens tabs in Chrome for Streamer and Viewer,
// and tests S4, S5, S6, S7, S8, S9, S10, S11 in a single orchestrated flow.
