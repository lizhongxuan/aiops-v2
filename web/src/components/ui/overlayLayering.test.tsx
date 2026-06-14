import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { Dialog, DialogContent, DialogDescription, DialogTitle } from "./dialog";
import { DropdownMenu, DropdownMenuContent } from "./dropdown-menu";
import { Popover, PopoverContent } from "./popover";

describe("overlay layering", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("renders modal and menu portal layers above the app shell header", async () => {
    await act(async () => {
      root.render(
        <>
          <header data-testid="app-shell-header" className="relative z-[70]">
            Header
          </header>
          <Dialog open>
            <DialogContent>
              <DialogTitle>Dialog title</DialogTitle>
              <DialogDescription>Dialog description</DialogDescription>
              Dialog content
            </DialogContent>
          </Dialog>
          <Popover open>
            <PopoverContent>Popover content</PopoverContent>
          </Popover>
          <DropdownMenu open>
            <DropdownMenuContent>Dropdown content</DropdownMenuContent>
          </DropdownMenu>
        </>,
      );
    });

    expect(document.body.querySelector('[data-slot="dialog-overlay"]')?.className).toContain("z-[90]");
    expect(document.body.querySelector('[data-slot="dialog-content"]')?.className).toContain("z-[90]");
    expect(document.body.querySelector('[data-slot="popover-content"]')?.className).toContain("z-[90]");
    expect(document.body.querySelector('[data-slot="dropdown-menu-content"]')?.className).toContain("z-[90]");
  });
});
