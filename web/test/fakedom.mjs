// Minimal DOM stand-in, just enough for the rendering modules under test.
// Supports simple selectors: tag, .class, [attr] and tag.class.

class FakeNode {
  constructor(tagName, namespaceURI = 'http://www.w3.org/1999/xhtml') {
    this.tagName = tagName;
    this.namespaceURI = namespaceURI;
    this.attributes = new Map();
    this.children = [];
    this.parentNode = null;
    this.listeners = {};
    this.ownText = '';
    this.checked = false;
    this.value = '';
  }

  setAttribute(k, v) {
    this.attributes.set(k, String(v));
    if (k === 'value') {
      this.value = String(v);
    }
    if (k === 'checked') {
      this.checked = true;
    }
  }

  getAttribute(k) {
    return this.attributes.has(k) ? this.attributes.get(k) : null;
  }

  get className() {
    return this.getAttribute('class') ?? '';
  }

  set className(v) {
    this.setAttribute('class', v);
  }

  get classList() {
    const list = () => this.className.split(/\s+/).filter(Boolean);
    return {
      contains: (c) => list().includes(c),
      toggle: (c, force) => {
        const set = new Set(list());
        const on = force ?? !set.has(c);
        if (on) {
          set.add(c);
        } else {
          set.delete(c);
        }
        this.className = [...set].join(' ');
        return on;
      },
    };
  }

  get dataset() {
    return Object.fromEntries([...this.attributes]
      .filter(([k]) => k.startsWith('data-'))
      .map(([k, v]) => [k.slice(5).replace(/-(\w)/g, (_, c) => c.toUpperCase()), v]));
  }

  append(...nodes) {
    for (const n of nodes) {
      n.parentNode = this;
      this.children.push(n);
    }
  }

  replaceChildren(...nodes) {
    this.children = [];
    this.append(...nodes);
  }

  get textContent() {
    return this.ownText + this.children.map((c) => c.textContent).join('');
  }

  set textContent(v) {
    this.children = [];
    this.ownText = v;
  }

  addEventListener(type, fn) {
    (this.listeners[type] ??= []).push(fn);
  }

  dispatch(type, ev = {}) {
    const event = { type, target: this, defaultPrevented: false, preventDefault() { this.defaultPrevented = true; }, ...ev };
    for (let n = this; n; n = n.parentNode) {
      for (const fn of n.listeners[type] ?? []) {
        fn(event);
      }
    }
    return event;
  }

  matches(sel) {
    const m = /^(\w+)?(?:\.([\w-]+))?(?:\[([\w-]+)\])?$/.exec(sel);
    if (!m) {
      throw new Error(`unsupported selector ${sel}`);
    }
    const [, tag, cls, attr] = m;
    return (!tag || this.tagName === tag) && (!cls || this.classList.contains(cls)) && (!attr || this.attributes.has(attr));
  }

  closest(sel) {
    for (let n = this; n; n = n.parentNode) {
      if (n.matches?.(sel)) {
        return n;
      }
    }
    return null;
  }

  querySelectorAll(sel) {
    const out = [];
    const walk = (n) => {
      for (const c of n.children) {
        if (c.matches?.(sel)) {
          out.push(c);
        }
        walk(c);
      }
    };
    walk(this);
    return out;
  }

  querySelector(sel) {
    return this.querySelectorAll(sel)[0] ?? null;
  }
}

class FakeText extends FakeNode {
  constructor(text) {
    super('#text');
    this.ownText = text;
  }

  matches() {
    return false;
  }
}

export function installFakeDom() {
  globalThis.Node = FakeNode;
  globalThis.document = {
    createElement: (tag) => new FakeNode(tag),
    createElementNS: (ns, tag) => new FakeNode(tag, ns),
    createTextNode: (text) => new FakeText(text),
  };
  return { container: () => new FakeNode('div') };
}
