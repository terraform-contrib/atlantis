# Envío de notificaciones mediante webhooks

Es posible enviar notificaciones a sistemas externos siempre que se esté realizando un apply o se detecte drift.

Puede hacer solicitudes a cualquier endpoint HTTP o enviar mensajes directamente a su canal de Slack.

::: tip NOTE
Los eventos `apply` e `drift` son compatibles.
:::

## Configuración

Los webhooks se configuran en la [configuración del lado del servidor](server-configuration.md) de Atlantis.
Puede haber muchos webhooks: enviar notificaciones a distintos destinos o para diferentes
workspaces/branches. Aquí hay una configuración de ejemplo para enviar mensajes de Slack para cada apply:

```yaml
webhooks:
- event: apply
  kind: slack
  channel: my-channel-id
```

Si está desplegando Atlantis como un chart de Helm, esto se puede implementar mediante el parámetro `config` disponible para [personalizaciones del chart](https://github.com/runatlantis/helm-charts#customization):

```yaml
## Use Server Side Config,
## ref: https://www.runatlantis.io/docs/server-configuration.html
config: |
   ---
   webhooks:
     - event: apply
       kind: slack
       channel: my-channel-id
```

### Filtrar por workspace/branch

Para limitar las notificaciones a workspaces o branches particulares, use los parámetros `workspace-regex` o `branch-regex`.
Si el workspace **y** la branch coinciden con su regex respectiva, se enviará un evento. Tenga en cuenta que una expresión regular vacía
(un resultado de un parámetro no establecido) coincide con cualquier cadena.

## Uso de webhooks HTTP

Puede enviar solicitudes POST con payload JSON a cualquier servidor HTTP/HTTPS.

### Configuración de Atlantis

En su [configuración del lado del servidor](server-configuration.md) de Atlantis puede agregar lo siguiente:

```yaml
webhooks:
- event: apply
  kind: http
  url: https://example.com/hooks
```

La información del evento `apply` se enviará por POST a `https://example.com/hooks`.

Puede proporcionar cualquier header adicional con el parámetro `--webhook-http-headers` (o variable de entorno),
por ejemplo con fines de autenticación. Vea [webhook-http-headers](server-configuration.md#webhook-http-headers) para más detalles.

### JSON payload

El payload es una struct [ApplyResult](https://pkg.go.dev/github.com/runatlantis/atlantis/server/events/webhooks#ApplyResult) serializada como JSON.

Payload de ejemplo:

```json
{
  "Workspace": "default",
  "Repo": {
    "FullName": "octocat/Hello-World",
    "Owner": "octocat",
    "Name": "Hello-World",
    "CloneURL": "https://:@github.com/octocat/Hello-World.git",
    "SanitizedCloneURL": "https://:<redacted>@github.com/octocat/Hello-World.git",
    "VCSHost": {
      "Hostname": "github.com",
      "Type": 0
    }
  },
  "Pull": {
    "Num": 2137,
    "HeadCommit": "7fd1a60b01f91b314f59955a4e4d4e80d8edf11d",
    "URL": "https://github.com/octocat/Hello-World/pull/2137",
    "HeadBranch": "feature/some-branch",
    "BaseBranch": "main",
    "Author": "octocat",
    "Body": "This is the pull request description.",
    "State": 0,
    "BaseRepo": {
      "FullName": "octocat/Hello-World",
      "Owner": "octocat",
      "Name": "Hello-World",
      "CloneURL": "https://:@github.com/octocat/Hello-World.git",
      "SanitizedCloneURL": "https://:<redacted>@github.com/octocat/Hello-World.git",
      "VCSHost": {
        "Hostname": "github.com",
        "Type": 0
      }
    }
  },
  "User": {
    "Username": "octocat",
    "Teams": null
  },
  "Success": true,
  "Directory": "terraform/example", 
  "ProjectName": "example-project"
}
```

Para eventos apply de pull requests, `Pull.HeadBranch` es la branch de origen del pull request e `Pull.Body` es la descripción del pull request cuando el proveedor VCS proporciona una.

## Uso de hooks de Slack

Para esto necesitará:

* Crear un usuario Bot en Slack
* Configurar Atlantis para enviar notificaciones a Slack.

### Configuración de Slack para Atlantis

* Vaya a [Slack: Apps](https://api.slack.com/apps)
* Haga clic en el botón `Create New App`
* Seleccione `From scratch` en el diálogo que se abre
* Asígnele un nombre, p. ej. `atlantis-bot`.
* Seleccione su workspace de Slack
* Haga clic en `Create App`
* A la izquierda vaya a `oAuth & Permissions`
* Desplácese hacia abajo hasta Scopes | Bot Token Scopes y agregue los siguientes alcances OAuth:
  * `channels:read`
  * `chat:write`
  * `groups:read`
  * `incoming-webhook`
  * `mpim:read`
* Instale la app en su workspace de Slack
* Copie el `Bot User OAuth Token` y proporciónelo a Atlantis usando `--slack-token=xoxb-xxxxxxxxxxx` o mediante el entorno `ATLANTIS_SLACK_TOKEN=xoxb-xxxxxxxxxxx`.
* Cree un canal en su workspace de Slack (p. ej. `my-channel`) o use uno existente
* Agregue la app al canal creado o al canal existente (haga clic en el nombre del canal, luego en la pestaña integrations, allí haga clic en "Add apps"

### Configuración de Atlantis

Después de seguir los pasos anteriores, es momento de configurar Atlantis. Suponiendo que ya ha proporcionado el `slack-token` (mediante parámetro o variable de entorno), ahora puede indicar a Atlantis que envíe eventos `apply` a Slack.

En su [configuración del lado del servidor](server-configuration.md) de Atlantis ahora puede agregar lo siguiente:

```yaml
webhooks:
- event: apply
  kind: slack
  channel: my-channel-id
```

Los mensajes de Slack de apply para pull requests incluyen un campo `Branch` usando la branch head del pull request, e incluyen un campo `Description` cuando el pull request tiene una descripción.

## Webhooks de detección de drift

Cuando la [detección de drift](api-endpoints.md#post-apidriftdetect) está habilitada (`--enable-drift-detection`), puede configurar webhooks para ser notificado siempre que la detección de drift se complete correctamente. Los webhooks de drift se envían automáticamente después de solicitudes `POST /api/drift/detect` exitosas, incluidos resultados heartbeat sin drift.

::: tip NOTE
Los webhooks de drift usan `event: drift` en la configuración del webhook. Son independientes de los webhooks `event: apply`.
:::

### Configuración de webhooks de drift

Los webhooks de drift se configuran junto con los webhooks de apply en el mismo bloque de configuración `webhooks`. Puede enviar notificaciones de drift a Slack, endpoints HTTP, o ambos:

```yaml
webhooks:
# Apply webhook (existing)
- event: apply
  kind: slack
  channel: apply-notifications

# Drift webhooks
- event: drift
  kind: slack
  channel: drift-alerts
- event: drift
  kind: http
  url: https://example.com/drift-webhook
```

::: tip NOTE
A diferencia de los webhooks de apply, los webhooks de drift no admiten filtrado `workspace-regex` o `branch-regex` porque la detección de drift opera a nivel de repositorio, no en el contexto de un pull request.
:::

### Formato del mensaje de drift en Slack

Cuando la detección de drift se completa correctamente, el mensaje de Slack incluye:

* **Color**: Rojo si se encontró drift, verde si no hay drift
* **Text**: "Drift detected in owner/repo" o "No drift in owner/repo"
* **Fields**: Atlantis, Repositorio, Ref, Proyectos con drift (cantidad), ID de detección. El campo Atlantis es el [`--atlantis-url`](server-configuration.md#atlantis-url) del servidor, para que pueda saber qué instancia de Atlantis envió el mensaje.
* **Drifted projects**: Una línea por cada proyecto con drift, con el nombre del proyecto, el directorio, el workspace y el resumen del plan. Se listan hasta 20 proyectos.

Por ejemplo, el campo de proyectos con drift contiene líneas como estas:

```text
• project: `vpc` dir: `modules/vpc` workspace: `production` — Plan: 1 to add, 2 to change, 0 to destroy.
• dir: `modules/iam` workspace: `default` — Note: Objects have changed outside of Terraform. Plan: 0 to add, 1 to change, 0 to destroy.
```

### Personalizar el mensaje de drift en Slack

Para reemplazar el mensaje predeterminado, configure `template` en un webhook de drift con `kind: slack`. La plantilla usa la sintaxis de [Go template](https://pkg.go.dev/text/template) con las [funciones de Sprig](https://masterminds.github.io/sprig/), excepto las que leen variables de entorno o resuelven hosts, y su resultado se envía como [Slack mrkdwn](https://api.slack.com/reference/surfaces/formatting). El mensaje conserva el color rojo o verde.

Por ejemplo, esta plantilla etiqueta los mensajes de un Atlantis de producción y omite las ejecuciones de detección que no encontraron drift:

```yaml
webhooks:
- event: drift
  kind: slack
  channel: drift-alerts
  template: |
    {{- if .ProjectsWithDrift -}}
    :rotating_light: *[production]* Drift detected in `{{ .Repository }}` (ref `{{ .Ref }}`) by {{ .AtlantisURL }}
    {{- range .Projects }}{{ if .HasDrift }}
    • {{ if .ProjectName }}project: `{{ .ProjectName }}` {{ end }}dir: `{{ .Path }}` workspace: `{{ .Workspace }}` — {{ .Summary }}
    {{- end }}{{ end }}
    {{- end -}}
```

La plantilla se renderiza con estos campos:

| Campo | Descripción |
| --- | --- |
| `.AtlantisURL` | URL del servidor de Atlantis que ejecutó la detección ([`--atlantis-url`](server-configuration.md#atlantis-url)) |
| `.Repository` | Nombre completo del repositorio, por ejemplo `owner/repo` |
| `.Ref` | Referencia de Git que se comprobó |
| `.DetectionID` | ID de la ejecución de detección |
| `.ProjectsWithDrift` | Cantidad de proyectos con drift |
| `.TotalProjects` | Cantidad de proyectos comprobados |
| `.Projects` | Los proyectos que se comprobaron |

Cada elemento de `.Projects` tiene estos campos:

| Campo | Descripción |
| --- | --- |
| `.ProjectName` | Nombre del proyecto en la configuración del repositorio. Vacío si el proyecto no tiene nombre. |
| `.Path` | Directorio del proyecto, relativo a la raíz del repositorio |
| `.Workspace` | Workspace de Terraform |
| `.HasDrift` | Si el proyecto tiene drift |
| `.ChangesOutside` | Si Terraform detectó objetos que cambiaron fuera de Terraform |
| `.ToAdd`, `.ToChange`, `.ToDestroy`, `.ToImport`, `.ToForget` | Cantidades de recursos del plan |
| `.Summary` | Resumen del plan en una línea, por ejemplo `Plan: 1 to add, 2 to change, 0 to destroy.` |

Tenga en cuenta lo siguiente sobre las plantillas:

* Los valores del resultado de la detección se escapan para Slack, por lo que no pueden agregar enlaces ni menciones. El texto de la propia plantilla se envía tal cual, por lo que puede incluir enlaces como `<https://example.com/runbook|runbook>` y menciones como `<!here>`.
* Si la plantilla solo genera espacios en blanco, no se envía ningún mensaje. El ejemplo anterior usa esto para omitir las ejecuciones de detección sin drift.
* Atlantis renderiza la plantilla con resultados de ejemplo, con y sin drift, al iniciarse, y no se inicia si la plantilla no es válida, por ejemplo porque usa un campo que no existe.
* `template` solo se admite en webhooks de drift con `kind: slack`.

### HTTP drift webhook payload

El webhook HTTP envía una solicitud POST con el siguiente payload JSON. `atlantis_url` es el [`--atlantis-url`](server-configuration.md#atlantis-url) del servidor, que identifica la instancia de Atlantis que envió el webhook.

```json
{
  "atlantis_url": "https://atlantis.example.com",
  "repository": "octocat/Hello-World",
  "ref": "main",
  "detection_id": "550e8400-e29b-41d4-a716-446655440000",
  "projects_with_drift": 1,
  "total_projects": 2,
  "projects": [
    {
      "project_name": "vpc",
      "path": "modules/vpc",
      "workspace": "production",
      "has_drift": true,
      "changes_outside": false,
      "to_add": 1,
      "to_change": 2,
      "to_destroy": 0,
      "to_import": 0,
      "to_forget": 0,
      "summary": "Plan: 1 to add, 2 to change, 0 to destroy."
    },
    {
      "project_name": "ec2",
      "path": "modules/ec2",
      "workspace": "production",
      "has_drift": false,
      "changes_outside": false,
      "to_add": 0,
      "to_change": 0,
      "to_destroy": 0,
      "to_import": 0,
      "to_forget": 0,
      "summary": ""
    }
  ]
}
```

Los mismos headers `--webhook-http-headers` configurados para webhooks de apply también se envían con solicitudes de webhook de drift.
