"""Общий мок anthropic-клиента: настоящий API в тестах не дёргаем никогда."""


class FakeBlock:
    def __init__(self, text):
        self.type = "text"
        self.text = text


class FakeResponse:
    def __init__(self, text):
        self.content = [FakeBlock(text)]


class FakeMessages:
    def __init__(self, replies):
        self.replies = list(replies)
        self.calls = []

    def create(self, **kwargs):
        self.calls.append(kwargs)
        if not self.replies:
            raise AssertionError("лишний вызов LLM")
        return FakeResponse(self.replies.pop(0))


class FakeClient:
    def __init__(self, *replies):
        self.messages = FakeMessages(replies)

    @property
    def calls(self):
        return self.messages.calls
