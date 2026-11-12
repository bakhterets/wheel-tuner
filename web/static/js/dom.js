// Tiny DOM builders. Attributes are set with setAttribute (never innerHTML)
// so user-provided text cannot inject markup.

const SVG_NS = 'http://www.w3.org/2000/svg';

function build(node, attrs, children) {
  for (const [k, v] of Object.entries(attrs ?? {})) {
    if (v === undefined || v === null || v === false) {
      continue;
    }
    if (k === 'text') {
      node.textContent = String(v);
    } else if (k === 'class') {
      node.setAttribute('class', Array.isArray(v) ? v.filter(Boolean).join(' ') : v);
    } else if (k.startsWith('on') && typeof v === 'function') {
      node.addEventListener(k.slice(2), v);
    } else {
      node.setAttribute(k, v === true ? '' : String(v));
    }
  }
  for (const c of [children ?? []].flat(Infinity)) {
    if (c !== null && c !== undefined && c !== false) {
      node.append(c instanceof Node ? c : document.createTextNode(String(c)));
    }
  }
  return node;
}

export function h(tag, attrs, children) {
  return build(document.createElement(tag), attrs, children);
}

export function s(tag, attrs, children) {
  return build(document.createElementNS(SVG_NS, tag), attrs, children);
}
