import { createSignal, type Accessor, type JSX } from "solid-js";

export type RouteName = string;

export type Route = {
  name: RouteName;
  render: () => JSX.Element;
};

const routes = new Map<RouteName, Route>();
const [active, setActive] = createSignal<RouteName | undefined>(undefined);

/** Register a route by name. Idempotent; re-registering replaces the
 *  prior renderer. Throws if name is empty. */
export function registerRoute(route: Route): void {
  if (route.name === "") throw new Error("router: empty route name");
  routes.set(route.name, route);
  if (active() === undefined) setActive(route.name);
}

/** activeRoute returns the currently selected route, or undefined when
 *  none is registered yet. Reactive — components can use it directly. */
export const activeRoute: Accessor<Route | undefined> = () => {
  const name = active();
  if (name === undefined) return undefined;
  return routes.get(name);
};

/** setActiveRoute switches the active route. Caller is responsible
 *  for ensuring the name has been registered first. */
export function setActiveRoute(name: RouteName): void {
  if (!routes.has(name)) throw new Error(`router: unknown route ${name}`);
  setActive(name);
}

/** routeNames returns a snapshot of currently-registered names.
 *  Map iteration preserves insertion order, so v8.0 sees ["agent"]
 *  and v8.1 will see ["agent", "harness"] in registration order. */
export function routeNames(): RouteName[] {
  return Array.from(routes.keys());
}
