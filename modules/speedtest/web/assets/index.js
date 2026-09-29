import { html, render, Component, useEffect, useMemo, useState } from 'https://lsong.org/scripts/react/index.js';
import { mountStandalone } from './runtime/standalone.js';

mountStandalone({ html, render, Component, useEffect, useMemo, useState });
