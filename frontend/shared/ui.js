import { h, Component } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import htm from 'htm';

export const html = htm.bind(h);
export { Component, h, useEffect, useMemo, useRef, useState };
