# Textual Message Bubbling: Sibling Communication

## Decision

Parent widgets must explicitly forward messages to sibling widgets. Do not use `@on(Message)` handlers on siblings.

## Why

Textual messages bubble **up** through the widget tree only (child → parent → grandparent). Sibling widgets never receive each other's messages. This caused a bug where `MRContainer` had `@on(RepoSelected)` but never received the message posted by `ReposContainer`.

## Pattern

```python
# WRONG: sibling handler — never fires
class MRContainer(Container):
    @on(RepoSelected)
    async def handle(self, msg): ...

# CORRECT: parent forwards explicitly
class LazyLabMainScreen(Screen):
    @on(RepoSelected)
    async def handle_repo_selection(self, message):
        self.selections.merge_requests.set_project(message.project)
```

## How to Apply

Any time a widget needs to react to an event from a sibling, the common parent must catch the event and call a method on the target sibling.
